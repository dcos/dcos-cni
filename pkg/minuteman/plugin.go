package minuteman

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
)

const DefaultPath = "/var/run/dcos/cni/l4lb"

type Error string

func (err Error) Error() string {
	return "minuteman: " + string(err)
}

func CniAdd(args *skel.CmdArgs) error {
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return Error(fmt.Sprintf("failed to load minuteman netconf: %s", err))
	}

	if conf.Path == "" {
		conf.Path = DefaultPath
	}

	// Create the directory where minuteman will search for the
	// registered containers.
	if err := os.MkdirAll(conf.Path, 0644); err != nil {
		return Error(fmt.Sprintf("couldn't create directory for storing minuteman container registration information:%s", err))
	}

	fmt.Fprintln(os.Stderr, "Registering netns for containerID", args.ContainerID, " at path: ", conf.Path)

	// Create a file with name `ContainerID` and write the network
	// namespace into this file.
	if err := ioutil.WriteFile(conf.Path+"/"+args.ContainerID, []byte(args.Netns), 0644); err != nil {
		return Error(fmt.Sprintf("couldn't checkout point the network namespace for containerID:%s for minuteman", args.ContainerID))
	}

	return nil
}

func CniDel(args *skel.CmdArgs) error {
	conf := &NetConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return Error(fmt.Sprintf("failed to load minuteman netconf: %s", err))
	}

	// For failures just log to `stderr` instead of  failing with an
	// error.
	if conf.Path == "" {
		conf.Path = DefaultPath
	}

	// Remove the container registration.
	if err := os.Remove(conf.Path + "/" + args.ContainerID); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to remove registration for contianerID:%s from minuteman", args.ContainerID)
	}

	return nil
}
