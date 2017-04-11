// Copyright 2015 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/asridharan/dcos-cni-plugins/pkg/spartan"

	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
)

type NetConf struct {
	types.NetConf
	Args     map[string]interface{} `json:"args"`
	MTU      int                    `json:"mtu"`
	Delegate map[string]interface{} `json:"delegate"`
}

func init() {
	// This ensures that main runs only on main thread (thread group leader).
	// Since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func setupContainerVeth(netns, ifName string, mtu int, pr *current.Result, spartanIPs []net.IPNet) (string, error) {
	// The IPAM result will be something like IP=192.168.3.5/24,
	// GW=192.168.3.1. What we want is really a point-to-point link but
	// veth does not support IFF_POINTOPONT. So we set the veth
	// interface to 192.168.3.5/32. Since the device netmask is set to
	// /32, this would not add any routes to the main routing table.
	// Therefore, in order to reach the spartan interfaces, we will have
	// to explicitly set routes to the spartan interface through this
	// device.

	var hostVethName string

	err := ns.WithNetNSPath(netns, func(hostNS ns.NetNS) error {
		hostVeth, _, err := ip.SetupVeth(ifName, mtu, hostNS)
		if err != nil {
			return err
		}

		containerVeth, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to lookup container VETH %q: %v", ifName, err)
		}

		// Configure the container veth with IP address returned by the
		// IPAM, but set the netmask to a /32. We are adding this closure
		// since we need modify the `types.Result` passed into this
		// method.
		err = func(res current.Result) error {
			if err := netlink.LinkSetUp(containerVeth); err != nil {
				return fmt.Errorf("failed to set %q UP: %v", ifName, err)
			}

			// Set the netmask to a /32.
			res.IPs[0].Address.Mask = net.IPv4Mask(0xff, 0xff, 0xff, 0xff)

			addr := &netlink.Addr{IPNet: &res.IPs[0].Address, Label: ""}
			if err = netlink.AddrAdd(containerVeth, addr); err != nil {
				return fmt.Errorf("failed to add IP address to %q: %v", ifName, err)
			}

			return nil
		}(*pr)

		if err != nil {
			return err
		}

		// Add routes to the spartan interfaces through this interface.
		for _, spartanIP := range spartanIPs {
			spartanRoute := netlink.Route{
				LinkIndex: containerVeth.Attrs().Index,
				Dst:       &spartanIP,
				Scope:     netlink.SCOPE_LINK,
				Src:       pr.IPs[0].Address.IP,
			}

			if err = netlink.RouteAdd(&spartanRoute); err != nil {
				return fmt.Errorf("failed to add spartan route %v: %v", spartanRoute, err)
			}
		}

		hostVethName = hostVeth.Name

		return nil
	})

	return hostVethName, err
}

func cmdAdd(args *skel.CmdArgs) error {
	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	if err := ip.EnableIP4Forward(); err != nil {
		return fmt.Errorf("failed to enable forwarding: %v", err)
	}

	// Invoke the delegate plugin.
	conf.Delegate["name"] = conf.Name
	conf.Delegate["cniVersion"] = conf.CNIVersion
	conf.Delegate["args"] = conf.Args

	delegateConf, err := json.Marshal(conf.Delegate)
	if err != nil {
		return fmt.Errorf("failed to marshall the delegate configuration: %v", err)
	}

	delegatePlugin, ok := conf.Delegate["type"].(string)
	if !ok {
		return fmt.Errorf("delegate plugin not defined in network: %s", conf.Delegate["name"])
	}

	delegateResult, err := invoke.DelegateAdd(delegatePlugin, delegateConf)
	if err != nil {
		return fmt.Errorf("failed to invoke delegate plugin %s: %v", delegatePlugin, err)
	}

	// Delegate plugin seems to be successful, install the spartan
	// network.
	spartanNetConf, err := json.Marshal(spartan.Config)
	if err != nil {
		return fmt.Errorf("failed to marshall the `spartan-network` IPAM configuration: %v", err)
	}

	// Run the IPAM plugin for the spartan network.
	ipamResult, err := ipam.ExecAdd(spartan.Config.IPAM.Type, spartanNetConf)
	if err != nil {
		return err
	}

	result, err := current.NewResultFromResult(ipamResult)
	if err != nil {
		return err
	}

	if result.IPs == nil {
		return errors.New("IPAM plugin returned missing IPv4 config")
	}

	// Make sure we got only one IP and that it is IPv4
	if (len(result.IPs) > 1) || result.IPs[0].Address.IP.To4() == nil {
		return errors.New("Expecting a single IPv4 address from IPAM")
	}

	hostVethName, err := setupContainerVeth(args.Netns, spartan.Config.Interface, conf.MTU, result, spartan.IPs)
	if err != nil {
		return err
	}

	hostVeth, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return fmt.Errorf("failed to lookup host VETH %q: %v", hostVethName, err)
	}

	containerRoute := netlink.Route{
		LinkIndex: hostVeth.Attrs().Index,
		Dst: &net.IPNet{
			IP:   result.IPs[0].Address.IP,
			Mask: net.IPv4Mask(0xff, 0xff, 0xff, 0xff),
		},
		Scope: netlink.SCOPE_LINK,
	}

	if err = netlink.RouteAdd(&containerRoute); err != nil {
		return fmt.Errorf("failed to add spartan route %v: %v", containerRoute, err)
	}

	//TODO(asridharan): We probably need to update the DNS result to
	//make sure that we override the DNS resolution with the spartan
	//network, since the operator has explicitly requested to use the
	//spartan network.

	// We always return the result from the delegate plugin and not from
	// this plugin.
	return delegateResult.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	spartanNetConf, err := json.Marshal(spartan.Config)
	if err != nil {
		return fmt.Errorf("failed to marshall the `spartan-network` IPAM configuration: %v", err)
	}

	if err = ipam.ExecDel(spartan.Config.IPAM.Type, spartanNetConf); err != nil {
		return err
	}

	if args.Netns == "" {
		return nil
	}

	// Ideally, the kernel would clean up the veth and routes within the
	// network namespace when the namespace is destroyed. We are still
	// explicitly deleting the interface here since we want the IP
	// address associated with the interface. We will then use the IP
	// address to clean up any associated routes in the host network
	// namespace. We also don't want any interfaces to be
	// present when the delegate plugin is invoked, since the presence
	// of these routes might confuse the delegate plugin.
	var ipn *net.IPNet
	err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		// We just need to delete the interface, the associated routes
		// will get deleted by themselves.
		var err error
		ipn, err = ip.DelLinkByNameAddr(spartan.Config.Interface, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete spartan interface in container: %v", err)
	}

	// Invoke the delegate plugin.
	conf.Delegate["name"] = conf.Name
	conf.Delegate["cniVersion"] = conf.CNIVersion
	conf.Delegate["args"] = conf.Args

	delegateConf, err := json.Marshal(conf.Delegate)
	if err != nil {
		return fmt.Errorf("failed to marshall the delegate configuration: %v", err)
	}

	delegatePlugin, ok := conf.Delegate["type"].(string)
	if !ok {
		return fmt.Errorf("delegate plugin not defined in network: %s", conf.Delegate["name"])
	}

	err = invoke.DelegateDel(delegatePlugin, delegateConf)
	if err != nil {
		return fmt.Errorf("failed to invoke delegate plugin %s: %v", delegatePlugin, err)
	}

	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}
