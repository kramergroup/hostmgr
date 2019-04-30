package app

import (
	"fmt"
	"net"
	"os/user"
)

//GetOutboundIP Get preferred outbound ip of this machine
func GetOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}

//Whoami returns the current username
func whoami() string {
	u, err := user.Current()
	if err != nil {
		log.Debug(fmt.Sprintf("Cannot determine username [%s]", err.Error()))
		return "unknown"
	}

	return u.Username
}
