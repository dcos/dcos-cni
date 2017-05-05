package main

import (
	"io/ioutil"
	"net"
	"os"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/testutils"
	"github.com/dcos/dcos-cni/pkg/minuteman"
	"github.com/dcos/dcos-cni/pkg/spartan"

	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("L4lb", func() {
	type L4lbCase struct {
		Conf        string
		Spartan     bool
		Minuteman   bool
		ContainerID string
		Path        string
	}

	var originalNS ns.NetNS
	const spartanIfName = "spartan"

	BeforeEach(func() {
		// Create a new NetNS so we don't modify the host
		var err error
		originalNS, err = ns.NewNS()
		Expect(err).NotTo(HaveOccurred())

		// Create dummy spartan interface in this namespace.
		dummy := &netlink.Dummy{
			LinkAttrs: netlink.LinkAttrs{
				Name:  spartanIfName,
				Flags: net.FlagUp,
				MTU:   1500,
			},
		}

		err = netlink.LinkAdd(dummy)
		Expect(err).NotTo(HaveOccurred())

		// Bring the interface up.
		err = netlink.LinkSetUp(dummy)
		Expect(err).NotTo(HaveOccurred())

		for _, spartanIP := range spartan.IPs {
			// Assign a /32 sparatn IP to this interface.
			addr := &netlink.Addr{
				IPNet: &spartanIP,
				Label: "",
			}
			err = netlink.AddrAdd(dummy, addr)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		// Remove the spartan dummy interface.
		Expect(originalNS.Close()).To(Succeed())

		_, err := ip.DelLinkByNameAddr(spartanIfName, netlink.FAMILY_V4)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("The spartan minuteman tests",
		func(input L4lbCase) {
			const IFNAME = "eth0"
			conf := input.Conf

			By("Adding a CNI configuration enabling spartan network")

			targetNS, err := ns.NewNS()
			Expect(err).NotTo(HaveOccurred())
			defer targetNS.Close()

			args := &skel.CmdArgs{
				ContainerID: input.ContainerID,
				Netns:       targetNS.Path(),
				IfName:      IFNAME,
				StdinData:   []byte(conf),
			}

			// Execute the plugin with the ADD command, creating the veth
			// endpoints.
			By("Invoking ADD to attach container to spartan network")
			err = originalNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				_, _, err := testutils.CmdAddWithResult(targetNS.Path(), IFNAME, []byte(conf), func() error {
					return cmdAdd(args)
				})
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if container has the spartan and minuteman interfaces configured")
			err = targetNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				// Check if the spartan link has been added.
				link, err := netlink.LinkByName(spartan.IfName)
				if input.Spartan {
					Expect(err).NotTo(HaveOccurred())
					Expect(link.Attrs().Name).To(Equal(spartan.IfName))
				} else {
					Expect(err).To(HaveOccurred())
				}

				// Check if the minuteman link has been added.
				link, err = netlink.LinkByName(minuteman.IfName)
				if input.Minuteman {
					Expect(err).NotTo(HaveOccurred())
					Expect(link.Attrs().Name).To(Equal(minuteman.IfName))
				} else {
					Expect(err).To(HaveOccurred())
				}

				// TODO(asridharan): Run the ping command for each of the spartan IP.
				return nil
			})

			By("Checking if plugin has registered network namespace with minuteman")
			netns, err := ioutil.ReadFile(input.Path + "/" + input.ContainerID)
			if input.Minuteman {
				Expect(err).NotTo(HaveOccurred())
				Ω(string(netns)).Should(Equal(targetNS.Path()))
			} else {
				Expect(err).To(HaveOccurred())
			}

			// Call the plugins with the DEL command, deleting the veth
			// endpoints.
			By("Invoking DEL to detach container from the spartan network")
			err = originalNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				err := testutils.CmdDelWithResult(targetNS.Path(), IFNAME, func() error {
					return cmdDel(args)
				})
				Expect(err).NotTo(HaveOccurred())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Make sure spartan and minuteman links have been deleted
			By("Checking that the spartan interface has been removed from the container netns")
			err = targetNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				link, err := netlink.LinkByName(spartan.IfName)
				Expect(err).To(HaveOccurred())
				Expect(link).To(BeNil())

				link, err = netlink.LinkByName(minuteman.IfName)
				Expect(err).To(HaveOccurred())
				Expect(link).To(BeNil())
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the network namespace has been de-registered from minuteman")
			_, err = os.Stat(input.Path + "/" + input.ContainerID)
			Ω(os.IsNotExist(err)).Should(BeTrue())
		},
		Entry("Default values",
			L4lbCase{
				Conf: `{
					"cniVersion": "0.2.0",
					"name": "spartan-net",
					"type": "dcos-l4lb",
					"delegate" : {
						"type" : "bridge",
						"bridge": "mesos-cni0",
						"ipMasq": true,
						"mtu": 5000,
						"ipam": {
							"type": "host-local",
							"subnet": "10.1.2.0/24",
							"routes": [
							{ "dst": "0.0.0.0/0" }
							]
						}
					}
				}`,
				Spartan:     true,
				Minuteman:   true,
				Path:        minuteman.DefaultPath,
				ContainerID: "dummy"}),
		Entry("Explicit values",
			L4lbCase{
				Conf: `{
					"cniVersion": "0.2.0",
					"name": "spartan-net",
					"type": "dcos-l4lb",
					"spartan": {
						"enable": true
					},
					"minuteman": {
						"path": "/tmp/minuteman_cni_test"
					},
					"delegate" : {
						"type" : "bridge",
						"bridge": "mesos-cni0",
						"ipMasq": true,
						"mtu": 5000,
						"ipam": {
							"type": "host-local",
							"subnet": "10.1.2.0/24",
							"routes": [
							{ "dst": "0.0.0.0/0" }
							]
						}
					}
				}`,
				Spartan:     true,
				Minuteman:   true,
				Path:        "/tmp/minuteman_cni_test",
				ContainerID: "dummy"}),
		Entry("Spartan Disabled",
			L4lbCase{
				Conf: `{
					"cniVersion": "0.2.0",
					"name": "spartan-net",
					"type": "dcos-l4lb",
					"spartan": {
						"enable": false
					},
					"minuteman": {
						"enable": true
					},
					"delegate" : {
						"type" : "bridge",
						"bridge": "mesos-cni0",
						"ipMasq": true,
						"mtu": 5000,
						"ipam": {
							"type": "host-local",
							"subnet": "10.1.2.0/24",
							"routes": [
							{ "dst": "0.0.0.0/0" }
							]
						}
					}
				}`,
				Spartan:     false,
				Minuteman:   true,
				Path:        minuteman.DefaultPath,
				ContainerID: "dummy"}),
	)
})
