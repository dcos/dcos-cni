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
	"fmt"
	"os"
	"runtime"

	"github.com/dcos/dcos-cni/pkg/minuteman"
	"github.com/dcos/dcos-cni/pkg/spartan"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
)

// We need this struct for de-duplicating the embedding of
// `types.NetConf` and `minuteman.NetConf` in `NetConf`.
type NetConf struct {
	types.NetConf
	Spartan   bool                   `json:"spartan, omitempty"`
	Minuteman *minuteman.NetConf     `json:"minuteman, omitempty"`
	Args      map[string]interface{} `json:"args, omitempty"`
	MTU       int                    `json:"mtu, omitempty"`
	Delegate  map[string]interface{} `json:"delegate, omitempty"`
}

// By default Spartan and Minuteman are specified to be enabled.

func init() {
	// This ensures that main runs only on main thread (thread group leader).
	// Since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func initConf() *NetConf {
	conf := &NetConf{
		Spartan: true,
		Minuteman: &minuteman.NetConf{
			Enable: true,
		},
	}

	return conf
}

func setupDelegateConf(conf *NetConf) (delegateConf []byte, delegatePlugin string, err error) {
	conf.Delegate["name"] = conf.Name
	conf.Delegate["cniVersion"] = conf.CNIVersion
	conf.Delegate["args"] = conf.Args

	delegateConf, err = json.Marshal(conf.Delegate)
	if err != nil {
		err = fmt.Errorf("failed to marshall the delegate configuration: %s", err)
		return
	}

	_, ok := conf.Delegate["type"]
	if !ok {
		err = fmt.Errorf("type field missing in delegate network: %s", conf.Delegate["name"])
		return
	}

	delegatePlugin, ok = conf.Delegate["type"].(string)
	if !ok {
		err = fmt.Errorf("type field in delegate network %s has incorrect type, expected a `string`", conf.Delegate["name"])
	}

	return
}

func cmdAdd(args *skel.CmdArgs) error {
	conf := initConf()

	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to load netconf: %s", err)
	}

	if err := ip.EnableIP4Forward(); err != nil {
		return fmt.Errorf("failed to enable forwarding: %s", err)
	}

	delegateConf, delegatePlugin, err := setupDelegateConf(conf)
	if err != nil {
		return fmt.Errorf("failed to retrieve delegate configuration: %s", err)
	}

	delegateResult, err := invoke.DelegateAdd(delegatePlugin, delegateConf)
	if err != nil {
		return fmt.Errorf("failed to invoke delegate plugin %s: %s", delegatePlugin, err)
	}

	if !conf.Spartan && conf.Minuteman == nil {
		return fmt.Errorf("at least one of minuteman or spartan CNI options need to be enabled for this plutin")
	}

	fmt.Fprintln(os.Stderr, "Spartan enabled:", conf.Spartan)
	if conf.Spartan {
		// Install the spartan network.
		err = spartan.CniAdd(args)
		if err != nil {
			return fmt.Errorf("failed to invoke the spartan plugin: %s", err)
		}

		//TODO(asridharan): We probably need to update the DNS result to
		//make sure that we override the DNS resolution with the spartan
		//network, since the operator has explicitly requested to use the
		//spartan network.
	}

	// Check if minuteman needs to be enabled for this container.
	fmt.Fprintln(os.Stderr, "Minuteman enabled:", conf.Minuteman.Enable)

	if conf.Minuteman.Enable {
		fmt.Fprintln(os.Stderr, "Asking plugin to register container netns for minuteman")
		minutemanArgs := *args
		minutemanArgs.StdinData, err = json.Marshal(conf.Minuteman)
		if err != nil {
			return fmt.Errorf("failed to marshal the minuteman configuration into STDIN for the minuteman plugin")
		}

		err = minuteman.CniAdd(&minutemanArgs)
		if err != nil {
			return fmt.Errorf("Unable to register container:%s with minuteman", args.ContainerID)
		}
	}

	// We always return the result from the delegate plugin and not from
	// this plugin.
	return delegateResult.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf := initConf()

	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to load netconf: %s", err)
	}

	if !conf.Spartan && conf.Minuteman == nil {
		return fmt.Errorf("one of minuteman or spartan need to be enabled for this plugin")
	}

	if conf.Spartan {
		err := spartan.CniDel(args)
		if err != nil {
			return fmt.Errorf("failed to invoke the spartan plugin with CNI_DEL")
		}
	}

	if conf.Minuteman != nil {
		var err error
		minutemanArgs := *args
		// Check if minuteman entries need to be removed from this container.
		minutemanArgs.StdinData, err = json.Marshal(conf.Minuteman)
		if err != nil {
			return fmt.Errorf("failed to marshal the minuteman configuration into STDIN for the minuteman plugin")
		}

		err = minuteman.CniDel(&minutemanArgs)
		if err != nil {
			return fmt.Errorf("Unable to register container:%s with minuteman", args.ContainerID)
		}
	}

	// Invoke the delegate plugin.
	delegateConf, delegatePlugin, err := setupDelegateConf(conf)
	if err != nil {
		return fmt.Errorf("failed to retrieve delegate configuration: %s", err)
	}

	err = invoke.DelegateDel(delegatePlugin, delegateConf)
	if err != nil {
		return fmt.Errorf("failed to invoke delegate plugin %s: %s", delegatePlugin, err)
	}

	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}
