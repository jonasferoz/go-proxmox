//go:build nodes
// +build nodes

package integration

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

var qcow = "https://download.opensuse.org/distribution/leap/15.3/appliances/openSUSE-Leap-15.3-JeOS.x86_64-OpenStack-Cloud.qcow2"

func downloadQcow() {
	root := strings.Split(os.Getenv("PROXMOX_USERNAME"), "@")
	if root[0] != "root" {
		log.Fatal("must use root")
	}

	config := &ssh.ClientConfig{
		User: root[0],
		Auth: []ssh.AuthMethod{
			ssh.Password(os.Getenv("PROXMOX_PASSWORD")),
		},

		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // insecure
	}

	u, err := url.Parse("PROXMOX_URL")
	if err != nil {
		fmt.Println("error parsing url:", err)
		return
	}

	url := u.Hostname() + ":22"

	client, err := ssh.Dial("tcp", url, config)
	if err != nil {
		log.Fatal("failed to dial: ", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatal("failed to create session: ", err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(fmt.Sprintf("/usr/bin/wget %s", qcow)); err != nil {
		log.Fatal("failed to run: " + err.Error())
	}
	fmt.Println(b.String())
}

func Test_NewQVM(t *testing.T) {
	downloadQcow()

	client := ClientFromLogins()
	node, err := client.Node(td.nodeName)
	require.NoError(t, err)

	cluster, err := client.Cluster()
	require.NoError(t, err)

	nextid, err := cluster.NextID()
	require.NoError(t, err)

	name := nameGenerator(7)

	task, err := node.NewVirtualMachine(nextid, proxmox.VirtualMachineOption{Name: "name", Value: name})
	require.NoError(t, err)
	require.NoError(t, task.Wait(1*time.Second, 10*time.Second))

	vm, err := node.VirtualMachine(nextid)
	require.NoError(t, err)

	task, err = vm.Config(

		proxmox.VirtualMachineOption{Name: "cores", Value: 2},
		proxmox.VirtualMachineOption{Name: "memory", Value: 4096},
		proxmox.VirtualMachineOption{Name: "net0", Value: "virtio,bridge=vmbr0"},
		proxmox.VirtualMachineOption{Name: "scsi0", Value: "local-lvm:0,import-from=/root/openSUSE-Leap-15.3-JeOS.x86_64-OpenStack-Cloud.qcow2,format=qcow2"},
		proxmox.VirtualMachineOption{Name: "scsihw", Value: "virtio-scsi-pci"},
		proxmox.VirtualMachineOption{Name: "boot", Value: "c"},
		proxmox.VirtualMachineOption{Name: "bootdisk", Value: "virtio0"},
		proxmox.VirtualMachineOption{Name: "agent", Value: 1},
		proxmox.VirtualMachineOption{Name: "vga", Value: "qxl"},
		proxmox.VirtualMachineOption{Name: "machine", Value: "q35"},
	)

	require.NoError(t, err)
	require.NoError(t, task.Wait(1*time.Second, 40*time.Second))

	// Start
	task, err = vm.Start()
	require.NoError(t, err)
	require.NoError(t, task.Wait(1*time.Second, 30*time.Second))
	require.NoError(t, vm.Ping())
	assert.Equal(t, proxmox.StatusVirtualMachineRunning, vm.Status)

	// Stop
	task, err = vm.Stop()
	assert.NoError(t, err)
	assert.NoError(t, task.Wait(1*time.Second, 15*time.Second))
	require.NoError(t, vm.Ping())
	assert.Equal(t, proxmox.StatusVirtualMachineStopped, vm.Status)
}
