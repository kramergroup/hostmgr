package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
)

const sshKnownHostPath string = "/etc/ssh/ssh_known_hosts"
const sshHostsEquivPath string = "/etc/ssh/shosts.equiv"

const privateHostKeyPath string = "/etc/ssh/ssh_host_rsa_key"
const publicHostKeyPath string = privateHostKeyPath + ".pub"

/*
HostDefinition describes a trusted host record
*/
type HostDefinition struct {
	Hostname   string `json:"hostname"`    // the hostname (should be DNS name)
	IP         string `json:"IP"`          // the host IP address
	PublicKey  string `json:"public_key"`  // the public host key
	ClientUser string `json:"client_user"` // the ssh client username (this is the account under which ssh runs, not the login name)
}

/*
Create creates a HostDefinition for the current host
*/
func Create() HostDefinition {

	var hostname string
	var publicKey []byte
	var ip net.IP
	var err error

	hostname, err = os.Hostname()
	if err != nil {
		hostname = ""
	}

	ip, err = GetOutboundIP()
	if err != nil {
		ip = net.IPv4(127, 0, 0, 1)
	}

	publicKey, err = ioutil.ReadFile(publicHostKeyPath)
	if err != nil {
		// attempt to create a public key file (will only work for root)
		cmd := exec.Command(
			"/usr/bin/ssh-keygen", "-t", "rsa",
			"-f", privateHostKeyPath,
			"-N", "",
			"-C", "hostmgr")

		errRun := cmd.Run()
		if errRun != nil {
			log.Error(fmt.Sprintf("Error creating host keys [%s]", errRun.Error()))
		} else {
			log.Info(fmt.Sprintf("Creating host keys in %s", privateHostKeyPath))
			publicKey, err = ioutil.ReadFile(publicHostKeyPath)
			if err != nil {
				log.Error("Error reading public host key file")
				publicKey = []byte("")
			}
		}
	}
	return HostDefinition{
		Hostname:   hostname,
		IP:         ip.String(),
		PublicKey:  string(publicKey),
		ClientUser: whoami(),
	}
}

/*
UpdateSSHConfiguration updates the hosts ssh configuration to
account for the provided host definitions
*/
func UpdateSSHConfiguration(defs []HostDefinition) error {

	log.Info("Updating SSH configuration")
	err := updateHostbasedAuthentication(defs)
	return err

}

/*
updateHostbasedAuthentication ensures that all provided host
definitions will be valid for host-based sshauthentication.

It does that by:

1) updating hosts.equiv with hostnames, IP addresses and ssh client usernames
2) make sure that the hosts public keys are stored in ssh_known_hosts

All other (non-changing) configuration is expected to be done separately
*/
func updateHostbasedAuthentication(defs []HostDefinition) error {

	err := writeKnowHostsFile(defs)
	if err != nil {
		log.Error(fmt.Sprintf("Error writing known host file. [%s]", err.Error()))
		return err
	}
	err = writeHostsEquivFile(defs)
	if err != nil {
		log.Error(fmt.Sprintf("Error writing known host file. [%s]", err.Error()))
		return err
	}
	return nil
}

/*
writeKnownHostFile updates the systems known host file (/etc/ssh/ssh_known_hosts)
*/
func writeKnowHostsFile(defs []HostDefinition) error {

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeOp := func(w io.Writer) error {
		var errW error
		// write records
		for _, def := range defs {
			entry := fmt.Sprintf("%s,%s %s\n", def.Hostname, def.IP, def.PublicKey)
			_, errW = w.Write([]byte(entry))
			if errW != nil {
				log.Debug(errW.Error())
			}
		}
		return errW
	}

	/*
		Read entries from ssh_known_hosts ignoring everything between "# hostmgr" comment
	*/
	f, err := os.Open(sshKnownHostPath)
	if err != nil {
		log.Debug(fmt.Sprintf("Error opening %s. Will attempt to create new file. Entries might be lost.", sshKnownHostPath))
		err = writeBetweenTags("# hostmgr", nil, w, writeOp)
	} else {
		// Splits on newlines by default.
		scanner := bufio.NewScanner(f)
		err = writeBetweenTags("# hostmgr", scanner, w, writeOp)
		f.Close() // Close the file before attempting to write to it
	}

	if err != nil {
		/*
			Even if an error has occured we attempt writing out the file and only
			log the error. This way, some entries migth make it to the file.
		*/
		log.Error("Error processing known hosts file. Entries might be lost.")
	}

	/*
		Write buffer to ssh_known_hosts file after flushing the writer
	*/
	w.Flush()
	errW := ioutil.WriteFile(sshKnownHostPath, buf.Bytes(), 0644)

	return errW
}

func writeHostsEquivFile(defs []HostDefinition) error {

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeOp := func(w io.Writer) error {
		var errW error
		// write records
		for _, def := range defs {
			_, errW = w.Write([]byte(fmt.Sprintf("%s %s\n", def.Hostname, def.ClientUser)))
			_, errW = w.Write([]byte(fmt.Sprintf("%s %s\n", def.IP, def.ClientUser)))
		}
		return errW
	}

	/*
		Read entries from hosts.equiv ignoring everything between "# hostmgr" comment
	*/
	f, err := os.Open(sshHostsEquivPath)
	if err != nil {
		log.Info(fmt.Sprintf("Error opening %s. Will attempt to create new file. Entries might be lost.", sshHostsEquivPath))
		err = writeBetweenTags("# hostmgr", nil, w, writeOp)
	} else {

		// Splits on newlines by default.
		scanner := bufio.NewScanner(f)
		err = writeBetweenTags("# hostmgr", scanner, w, writeOp)
		f.Close() // Close the file before attempting to write to it
	}
	if err != nil {
		/*
			Even if an error has occured we attempt writing out the file and only
			log the error. This way, some entries migth make it to the file.
		*/
		log.Error("Error processing shosts.equiv file. Entries might be lost.")
	}

	/*
		Write buffer to shosts.equiv file after flushing the buffer
	*/
	w.Flush()
	errW := ioutil.WriteFile(sshHostsEquivPath, buf.Bytes(), 0644)

	return errW

}

/* writeBetweenTags is a helper function that scans a buffer for a tag and replaces the
content between occurances of the tag with
*/
func writeBetweenTags(tag string, scanner *bufio.Scanner, w io.Writer, callback func(io.Writer) error) error {

	var err error
	var blockWritten = false

	writeBlock := func() {
		_, err = w.Write([]byte(fmt.Sprintln(tag)))
		if err == nil {
			err = callback(w)
			if err == nil {
				_, err = w.Write([]byte(fmt.Sprintln(tag)))
			}
		}
		blockWritten = (err == nil)
	}

	if scanner != nil {
		pass := false
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), tag) {
				pass = !pass
				if !blockWritten {
					writeBlock()
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	/*
		Ensure that the block is written if the file is empty or does not contain a tag
	*/
	if !blockWritten {
		writeBlock()
	}

	return err
}
