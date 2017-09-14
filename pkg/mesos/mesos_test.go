package mesos_test

import (
	"github.com/dcos/dcos-cni/pkg/mesos"

	"net"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mesos", func() {
	Describe("Testing container IP", func() {
		var (
			hostIP net.IP
		)

		BeforeEach(func() {
			os.Unsetenv("LIBPROCESS_IP")
			os.Unsetenv("MESOS_CONTAINER_IP")

			hostName, _err := os.Hostname()
			Expect(_err).NotTo(HaveOccurred(), "Error while retrieving hostname: %s", _err)

			ipAddr, _err := net.ResolveIPAddr("ip", hostName)
			Expect(_err).NotTo(HaveOccurred(), "Error while resolving host IP address: %s", _err)

			hostIP = ipAddr.IP
		})

		Context("With `MESOS_CONTAINER_IP` set and `LIBPROCESS_IP` unset", func() {
			It("Testing `MESOS_CONTAINER_IP`", func() {
				os.Setenv("MESOS_CONTAINER_IP", "1.1.1.2")
				ip, err := mesos.ContainerIP()
				Expect(err).NotTo(HaveOccurred(), "Error while parsing `MESOS_CONTAINER_IP`: %s", err)
				Expect(ip).To(Equal(net.ParseIP("1.1.1.2")), "Couldn't get IP from `MESOS_CONTAINER_IP`: %s", ip)
			})
		})

		Context("With `LIBPROCESS_IP` set and `MESOS_CONTAINER_IP` unset", func() {
			It("Testing `LIBPROCESS_IP`", func() {
				os.Setenv("LIBPROCESS_IP", "1.1.1.3")
				ip, err := mesos.ContainerIP()
				Expect(err).NotTo(HaveOccurred(), "Error while parsing `LIBPROCESS_IP`: %s", err)
				Expect(ip).To(Equal(net.ParseIP("1.1.1.3")), "Couldn't get IP from `LIBPROCESS_IP`: %s", ip)
			})
		})

		Context("With `LIBPROCESS_IP` set and `MESOS_CONTAINER_IP` set", func() {
			It("Testing `LIBPROCESS_IP`", func() {
				os.Setenv("LIBPROCESS_IP", "1.1.1.4")
				os.Setenv("MESOS_CONTAINER_IP", "1.1.1.5")
				ip, err := mesos.ContainerIP()
				Expect(err).NotTo(HaveOccurred(), "Error while parsing `MESOS_CONTAINER_IP`: %s", err)
				Expect(ip).To(Equal(net.ParseIP("1.1.1.5")), "Couldn't get IP from `MESOS_CONTAINER_IP`: %s", ip)
			})
		})

		Context("With `LIBPROCESS_IP` set to 0.0.0.0 and `MESOS_CONTAINER_IP` unset", func() {
			It("Testing `LIBPROCESS_IP` set to INADDR_ANY", func() {
				os.Setenv("LIBPROCESS_IP", "0.0.0.0")
				ip, err := mesos.ContainerIP()
				Expect(err).NotTo(HaveOccurred(), "Error while parsing `hostIP`: %s", err)
				Expect(ip).To(Equal(hostIP), "Couldn't get IP from hostname: %s", ip)
			})
		})

	})

})
