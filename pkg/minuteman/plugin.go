package minuteman

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
)

func CniAdd(args *skel.CmdArgs) error {
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to load minuteman netconf: %s", err)
	}

	if conf.Minuteman == nil {
		return fmt.Errorf("missing field minuteman")
	}

	if conf.Minuteman.Path == "" {
		return fmt.Errorf("missing path for minuteman state")
	}

	// Create the directory where minuteman will search for the
	// registered containers.
	if err := os.MkdirAll(conf.Minuteman.Path, 0644); err != nil {
		return fmt.Errorf("couldn't create directory for storing minuteman container registration information:%s", err)
	}

	// Create a file with name `ContainerID` and write the network
	// namespace into this file.
	if err := ioutil.WriteFile(conf.Minuteman.Path+"/"+args.ContainerID, []byte(args.Netns), 0644); err != nil {
		return fmt.Errorf("couldn't checkout point the network namespace for containerID:%s for minuteman", args.ContainerID)
	}

	return nil
}

func CniDel(args *skel.CmdArgs) error {
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to load minuteman netconf: %s", err)
	}

	// For failures just log to `stderr` instead of  failing with an
	// error.
	if conf.Minuteman == nil {
		fmt.Fprintf(os.Stderr, "missing field minuteman")
	}

	if conf.Minuteman.Path == "" {
		fmt.Fprintf(os.Stderr, "missing path for minuteman state")
	}

	// Remove the container registration.
	if err := os.Remove(conf.Minuteman.Path + "/" + args.ContainerID); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to remove registration for contianerID:%s from minuteman", args.ContainerID)
	}

	return nil
}
