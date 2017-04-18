package main

import (
	"net"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/testutils"
	"github.com/dcos/dcos-cni/pkg/spartan"

	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("L4lb", func() {
	var originalNS ns.NetNS
	var spartanIfName = "spartan"

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

	// Spartan tests.
	It("configures and deconfigures a spartan on a bridge network with ADD/DEL", func() {
		const IFNAME = "eth0"

		By("Adding a CNI configuration enabling spartan network")
		conf := `{
    	"cniVersion": "0.2.0",
    	"name": "spartan-net",
    	"type": "dcos-l4lb",
			"spartan": true,
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
		}`

		targetNS, err := ns.NewNS()
		Expect(err).NotTo(HaveOccurred())
		defer targetNS.Close()

		args := &skel.CmdArgs{
			ContainerID: "dummy",
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

		By("Checking if container has the spartan interfaces configured")
		err = targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			// Check if the spartan link has been added.
			link, err := netlink.LinkByName(spartan.IfName)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().Name).To(Equal(spartan.IfName))

			// Run the ping command for each of the spartan IP.
			return nil
		})

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

		// Make sure spartan link has been deleted
		By("Checking that the spartan interface has been removed from the container netns")
		err = targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			link, err := netlink.LinkByName(spartan.IfName)
			Expect(err).To(HaveOccurred())
			Expect(link).To(BeNil())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	// Minuteman tests.
	It("configures and deconfigures minuteman entries during  ADD/DEL", func() {
		const IFNAME = "eth0"

		By("Defining a minuteman configuration")

		conf := `{
    	"cniVersion": "0.2.0",
    	"name": "spartan-net",
    	"type": "dcos-l4lb",
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
		}`

		targetNs, err := ns.NewNS()
		Expect(err).NotTo(HaveOccurred())
		defer targetNs.Close()

		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       targetNs.Path(),
			IfName:      IFNAME,
			StdinData:   []byte(conf),
		}

		// Execute the plugin with the ADD command, creating the veth
		// endpoints.
		By("Invoking ADD on the plugin for registering netns with minuteman")
		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			_, _, err := testutils.CmdAddWithResult(targetNs.Path(), IFNAME, []byte(conf), func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		// Check if the container has been checkpointed.

		// Call the plugins with the DEL command, deleting the veth
		// endpoints.
		By("Invoking DEL on the plugin for de-registering netns with minuteman")
		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			err := testutils.CmdDelWithResult(targetNs.Path(), IFNAME, func() error {
				return cmdDel(args)
			})
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
