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
	"runtime"

	"github.com/dcos/dcos-cni/pkg/spartan"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
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
	conf := &NetConf{}
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

	// Delegate plugin seems to be successful, install the spartan
	// network.

	err = spartan.CniAdd(args)
	if err != nil {
		return fmt.Errorf("failed to invoke the spartan plugin: %s", err)
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
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to load netconf: %s", err)
	}

	err := spartan.CniDel(args)
	if err != nil {
		return fmt.Errorf("failed to invoke the spartan plugin with CNI_DEL")
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
