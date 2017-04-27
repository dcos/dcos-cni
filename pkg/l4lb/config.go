package l4lb

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"

	"github.com/dcos/dcos-cni/pkg/minuteman"
)

type NetConf struct {
	types.NetConf
	Spartan   bool                   `json:"spartan, omitempty"`
	Minuteman *minuteman.NetConf     `json:"minuteman, omitempty"`
	Args      map[string]interface{} `json:"args, omitempty"`
	MTU       int                    `json:"mtu, omitempty"`
	Delegate  map[string]interface{} `json:"delegate, omitempty"`
}

func NewNetConf() *NetConf {
	conf := &NetConf{
		Spartan: true,
		Minuteman: &minuteman.NetConf{
			Enable: true,
		},
	}

	return conf
}

func (conf *NetConf) SetupDelegateConf() (delegateConf []byte, delegatePlugin string, err error) {
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
