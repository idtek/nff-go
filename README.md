[![Go Report Card](https://goreportcard.com/badge/github.com/intel-go/yanff)](https://goreportcard.com/report/github.com/intel-go/yanff) 
[![GoDoc](https://godoc.org/github.com/intel-go/yanff?status.svg)](https://godoc.org/github.com/intel-go/yanff)
[![Dev chat at https://gitter.im/intel-yanff/Lobby](https://img.shields.io/badge/gitter-developer_chat-46bc99.svg)](https://gitter.im/intel-yanff/Lobby?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
[![Build Status](https://travis-ci.org/intel-go/nff-go.svg?branch=develop)](https://travis-ci.org/intel-go/nff-go)
# Network Function Framework for Go (former YANFF)

**To our users: if you are using NFF-Go, can you please send a message to [us](mailto:areg.melik-adamyan@intel.com) with a function name/type that you are using. This will help us to determine better roadmap. Thank you!**

## What it is
NFF-Go is a set of libraries for creating and deploying cloud-native Network
Functions (NFs). It simplifies the creation of network functions without
sacrificing performance. 
* Higher level abstractions than DPDK. Using DPDK as a fast I/O engine for performance
* Go language: safety, productivity, performance, concurrency
* Network functions are application programs not virtual machines
* Built-in scheduler to auto-scale processing based on input traffic. Both up and down.

### Benefits:
* Easily leverage Intel hardware capabilities: multi-cores, AES-NI, CAT, QAT, DPDK
* 10x reduction in lines of code
* No need to be an expert network programmer to develop performant network function
* Similar performance with C/DPDK per box 
* No need to worry on elasticity - done automatically
* Take advantage of cloud native deployment: continuous delivery, micro-services, containers

### Feel the difference
Simple ACL based firewall
```Go

func main() {
	// Initialize NFF-GO library to use 8 cores max.
	config := flow.Config{
		CPUCoresNumber: 8,
	}
	flow.CheckFatal(flow.SystemInit(&config))

	// Get filtering rules from access control file.
	L3Rules, err := packet.GetL3ACLFromORIG("Firewall.conf")
	flow.CheckFatal(err)

	// Receive packets from zero port. Receive queue will be added automatically.
	inputFlow, err := flow.SetReceiver(uint8(0))
	flow.CheckFatal(err)

	// Separate packet flow based on ACL.
	rejectFlow, err := flow.SetSeparator(inputFlow, L3Separator, nil)
	flow.CheckFatal(err)

	// Drop rejected packets.
	flow.CheckFatal(flow.SetStopper(rejectFlow))

	// Send accepted packets to first port. Send queue will be added automatically.
	flow.CheckFatal(flow.SetSender(inputFlow, uint8(1)))

	// Begin to process packets.
	flow.CheckFatal(flow.SystemStart())
}

// User defined function for separating packets
func L3Separator(currentPacket *packet.Packet, context flow.UserContext) bool {
	currentPacket.ParseL4()
	// Return whether packet is accepted or not. Based on ACL rules.
	return currentPacket.L3ACLPermit(L3Rules)
}
```
NFF-GO is an Open Source BSD licensed project that runs mostly in Linux user
land. The most recent patches and enhancements provided by the community are
available in the *_develop_* branch. master branch provides the latest stable released version under the appropriate tag. 

## Getting NFF-GO

Starting with release 0.7.0 NFF-Go uses go.mod for getting dependencies,
therefore Go version 1.11 or later is required. To checkout NFF-Go
sources use the following command

        git clone --recurse-submodules http://github.com/intel-go/nff-go

### Working with a github fork

If you are working on a fork, then the **go get** command will not put nff-go in
$GOPATH/src/github.com/intel-go. However, imports will continue to reference
github.com/intel-go. This is a feature of Go and not a problem in the way nff-go
is written. See [stackoverflow
article](https://stackoverflow.com/questions/14323872/using-forked-package-import-in-go)
for a discussion. A simple way to resolve the problem is to use a symlink. If
you are rscohn2 on github, and you forked nff-go into your personal account,
then do this:

        cd $GOPATH/src/github.com
        mkdir intel-go
        cd intel-go
        ln -s ../rscohn2/nff-go .

## Setting up the build and run environment

### DPDK
    
NFF-GO uses DPDK, so you must setup your system to build and run DPDK. See [System
Requirements in the DPDK Getting Started Guide for
Linux](http://dpdk.org/doc/guides/linux_gsg/sys_reqs.html) for more
information.

After building a DPDK driver with the make command, you must register network
cards to work with the DPDK driver, load necessary kernel modules, and bind
cards to the modules. See [Compiling the DPDK Target from
Source](http://dpdk.org/doc/guides/linux_gsg/build_dpdk.html) and [How to get
best performance with NICs on Intel
platforms](http://dpdk.org/doc/guides/linux_gsg/nic_perf_intel_platform.html)
in the DPDK Getting Started Guide for Linux for more information.

The kernel module, which is required for DPDK user-mode drivers, is built but
not installed into kernel directory. You can load it using the full path to the
module file:
$GOPATH/src/github.com/intel-go/nff-go/test/dpdk/dpdk-17.08/x86_64-native-linuxapp-gcc/kmod/igb_uio.ko

### Go

Use Go version 1.11 or higher. To check the version of Go, do:

        go version
        
## Building NFF-GO

        cd $GOPATH/src/github.com/intel-go/nff-go
        make -j8

## Building NFF-GO in debug mode

		make debug -j8

# Running NFF-GO

## Documentation 

Use:

        make doc

to generate full documentation. Alternatively, you can do:

        godoc -http=:6060

and browse the following URLs:

* http://localhost:6060/pkg/nff-go/flow/
* http://localhost:6060/pkg/nff-go/packet/

## Tests

Invoking make in the top-level directory builds the testing framework and
examples. NFF-GO distributed tests are packaged inside of Docker container
images. There are also single node unit tests in some packages that you can
run using the command:

         make testing

### Docker images

To create Docker images on the local default target (either the default UNIX
socket in /var/run/docker.sock or whatever is defined in the DOCKER_HOST
variable), use the **make images** command.

To deploy Docker images for use in distributed testing, use the **make deploy**
command. This command requires two environment variables:

* NFF_GO_HOSTS="hostname1 hostname2 ... hostnameN"* - a list of all hostnames for deployed test Docker images
* DOCKER_PORT=2375* - the port number to connect to Docker daemons running on hosts in the NFF_GO_HOSTS variable

To delete generated images in the default Docker target, use the **make
clean-images** command.

### Running tests

After the Docker images are deployed on all test hosts, you can run distributed
network tests. The test framework is located in the test/main directory and
accepts a JSON file with a test specification. There are predefined configs for
performance and stability tests in the same directory. To run these tests,
change **hostname1** and **hostname2** to the hosts from the NFF_GO_HOSTS list
in these JSON files.

## Cleaning-up

To clean all generated binaries, use the **make clean** command.  To delete all
deployed images listed in NFF_GO_HOSTS, use the **make cleanall** command.

## Changing the DPDK sources

If you use the **make** command from NFF-GO directories, the DPDK driver is
downloaded automatically.

## Contributing

If you want to contribute to NFF-Go, check our [Contributing
guide](https://github.com/intel-go/yanff/blob/master/CONTRIBUTING.md). We also
recommend checking the bugs with 'help-wanted' or 'easyfix' in our list of open issues; these bugs
can be solved without an extensive knowledge of NFF-Go. We would love to help
you start contributing.

You can reach the NFF-Go development team via our [mailing list](mailto:areg.melik-adamyan@intel.com).

    
