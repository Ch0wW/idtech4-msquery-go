package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"time"
)

var (
	link     string
	port     string
	mod      string
	protocol int
)

type idTech4_Server struct {
	IP   net.IP
	Port uint16
}

type QuakePacket struct {
	buf bytes.Buffer // Buffer to send
}

func (pkt *QuakePacket) WriteString(cmd string) {
	pkt.buf.Write([]byte(cmd))
	pkt.buf.WriteByte(0)
}

func (pkt *QuakePacket) WriteByte(cmd byte) {
	pkt.buf.WriteByte(cmd)
}

func (pkt *QuakePacket) PreparePacket() {
	pkt.buf.WriteByte(255)
	pkt.buf.WriteByte(255)
}

func (pkt *QuakePacket) WriteLong(packetsize uint32) {

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, packetsize)

	pkt.buf.Write(b)
}

func (pkt *QuakePacket) ExportToBytes() []byte {

	return pkt.buf.Bytes()
}

type QuakeAnswer struct {
	buffer    []byte
	bufferpos int
	bufferlen int
}

// ReadByte - Reads the byte.
// Moves 1 byte in the request position.
func (sv *QuakeAnswer) ReadByte() (byte, error) {

	if sv.bufferpos+1 > sv.bufferlen {
		errmsg := fmt.Sprintf("Buffer going too far! (pos: %d, size:%d)", sv.bufferpos+1, sv.bufferlen)
		return 0, errors.New(errmsg)
	}

	val := sv.buffer[sv.bufferpos]
	sv.bufferpos = sv.bufferpos + 1

	return val, nil
}

// ReadShort - Reads a short into the request list.
// Moves 2 bytes in the request position.
func (sv *QuakeAnswer) ReadShort() (uint16, error) {

	if sv.bufferpos+2 > sv.bufferlen {
		errmsg := fmt.Sprintf("Buffer going too far! (pos: %d, size:%d)", sv.bufferpos+2, sv.bufferlen)
		return 0, errors.New(errmsg)
	}

	test := binary.LittleEndian.Uint16(sv.buffer[sv.bufferpos:])
	value := uint16(test)
	sv.bufferpos = sv.bufferpos + 2

	return uint16(value), nil
}

// Transform the byte into a long.
func (sv *QuakeAnswer) ReadString() (string, error) {

	result := ""

	for true {
		c, err := sv.ReadByte()

		if err != nil {
			return "", err
		}

		if c <= 0 || c >= 255 {
			break
		}

		if c == '%' {
			c = '.'
		}

		result = result + string(c)
	}

	return result, nil
}

func QueryMasterServer() ([]idTech4_Server, error) {

	// Translate DNS into a readable IP
	daIP, err := net.LookupIP(link)
	if err != nil {
		fmt.Println("Unknown host")
	}
	ip := daIP[0].String()

	svlink := ip + ":" + port

	var pkt QuakePacket
	pkt.PreparePacket()
	pkt.WriteString("getServers")

	if protocol == 0 {
		pkt.WriteLong((1 << 16) + 41)
	} else if protocol == 1 {
		pkt.WriteLong(131157) // Quake 4 protocol (\x55\x00\x02\x80)
	} else if protocol == 2 {
		pkt.WriteLong((1 << 16) + 41 + 1)
	}
	pkt.WriteString(mod)
	pkt.WriteByte(0) // ?
	pkt.WriteByte(0) // ?
	pkt.WriteByte(0) // ?

	//Connect udp
	conn, err := net.DialTimeout("udp", svlink, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot access the server: %s", err)
	}
	defer conn.Close()

	// Query the server to check if we're a valid QW server
	_, err = conn.Write(pkt.ExportToBytes())
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("Write Timeout: %s", err)
		}
		return nil, fmt.Errorf("write Error: %s", err)
	}

	// Read the answer and trim it, so that empty bytes won't be displayed.
	buffer := make([]byte, 8196)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	buffersize, err := conn.Read(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, fmt.Errorf("read timeout: %s", err)
		}
		return nil, fmt.Errorf("read Error: %s", err)
	}

	if buffersize <= 0 {
		return nil, fmt.Errorf("server has no data to answer with")
	}

	a := QuakeAnswer{
		buffer:    buffer,
		bufferpos: 0,
		bufferlen: buffersize,
	}

	var list []idTech4_Server

	_, err = a.ReadShort()
	if err != nil {
		return nil, fmt.Errorf("Read Error: %s", err)
	}

	querytxt, err := a.ReadString()
	if err != nil {
		return nil, fmt.Errorf("Read Error: %s", err)
	}
	if querytxt != "servers" {
		return nil, fmt.Errorf("Unknown request: %s != servers ", querytxt)
	}

	for {

		ipa, err := a.ReadByte()
		if err != nil {
			break
		}

		ipb, err := a.ReadByte()
		if err != nil {
			break
		}

		ipc, err := a.ReadByte()
		if err != nil {
			break
		}

		ipd, err := a.ReadByte()
		if err != nil {
			break
		}

		ipport, err := a.ReadShort()
		if err != nil {
			break
		}

		servtoip := []byte{ipa, ipb, ipc, ipd}

		tempentry := idTech4_Server{
			IP:   net.IP(servtoip),
			Port: ipport,
		}

		list = append(list, tempentry)
	}

	return list, nil
}

func main() {

	flag.StringVar(&link, "ip", "", "URL of a custom idTech4 masterserver (default: none)")
	flag.StringVar(&port, "port", "27650", "Port of the masterserver (default: 27650)")
	flag.StringVar(&mod, "mod", "", "Filters the list with the mod requested.")
	flag.IntVar(&protocol, "protocol", 0, "Use the protocol for query (0: for Doom 3 & Prey, 1: Quake4, 2: DHEWM3). (default: 0)")
	flag.Parse()

	prot := ""
	if protocol == 0 {
		prot = "Doom 3 / Prey"
	} else if protocol == 1 {
		prot = "Quake 4"
	} else if protocol == 2 {
		prot = "DHEWM3"
	} else {
		prot = "Unknown choice, reverting to Doom3 / Prey."
		protocol = 0
	}

	if link == "" {
		if protocol == 0 || protocol == 2 {
			link = "idnet.ua-corp.com"
		} else if protocol == 1 {
			link = "q4master.idsoftware.com"
		}
	}

	fmt.Println("==========================")
	fmt.Println("iDTech4 MasterServer Query Tool")
	fmt.Println("Written by Ch0wW - https://ch0ww.fr")
	fmt.Println("")
	fmt.Println("Settings:")
	fmt.Println("- MasterServer Address:", link)
	fmt.Println("- Port:", port)
	fmt.Println("- Protocol:", prot)
	fmt.Println("==========================")

	list, err := QueryMasterServer()

	if err != nil {
		fmt.Println(err)
		return
	}

	for a := range list {

		sv := list[a]
		fmt.Printf("%s:%d\n", sv.IP, sv.Port)
	}

	fmt.Println("There are", len(list), "servers found.")

}
