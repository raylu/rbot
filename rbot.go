package main

import (
	irc "github.com/fluffle/goirc/client"
	"fmt"
	"os"
	"strings"
	"time"
	"net"
	"bufio"
	"crypto/tls"
	"crypto/rand"
	"goconfig"
)

const confFile = "rbot.conf"
var trigger string
var sections []string
var conf *config.Config

func main() {
	readConf()
	trigger = readConfString("DEFAULT", "trigger")
	readAuth()

	identdport, _ := conf.String("DEFAULT", "identdport")
	if identdport != "" && identdport != "0" {
		go identd(identdport)
	}

	sections = conf.Sections()
	for _, s := range sections {
		if strings.Index(s, " ") == -1 && s != "DEFAULT" {
			// found a network
			go connect(s)
		}
	}

	select { }
}

func connect(network string) {
	if !readConfBool(network, "autoconnect") {
		return
	}
	server := readConfString(network, "server")
	nick := readConfString(network, "nick")
	user := readConfString(network, "user")
	nickserv, _ := conf.String(network, "nickserv")

	c := irc.New(nick, user, user)
	c.Network = network
	c.SSL = readConfBool(network, "ssl")
	if c.SSL {
		// we don't care about certificate validity
		c.SSLConfig = &tls.Config{Rand: rand.Reader, Time: time.Nanoseconds}
	}

	c.AddHandler("connected",
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Printf("Connected to %s!\n", conn.Host)

			if len(nickserv) > 0 {
				conn.Privmsg("NickServ", "IDENTIFY " + nickserv)
			} else {
				autojoin(conn)
			}
		})
	c.AddHandler("privmsg", handlePrivmsg)
	c.AddHandler("mode", handleMode)
	c.AddHandler("join", handleJoin)
	c.AddHandler("invite", handleInvite)

	for {
		fmt.Printf("Connecting to %s...\n", server)
		if err := c.Connect(server); err != nil {
			fmt.Printf("Connection error: %s\n", err)
		}
		for c.Connected {
			if err := <-c.Err; err != nil {
				fmt.Printf("goirc error: %s", err)
			}
		}
		time.Sleep(30000000000) // 30 seconds
	}
}

func autojoin(conn *irc.Conn) {
	for _, s := range sections {
		split := strings.SplitN(s, " ", 2)
		if len(split) == 2 && split[0] == conn.Network {
			// found a channel
			if readConfBool(s, "autojoin") {
				fmt.Printf("Joining %s on %s\n", split[1], conn.Network)
				conn.Join(split[1])
			}

			if annstream, _ := conf.Bool(s, "stream"); annstream {
				announceJustinTv(conn, split[1])
			}
		}
	}
}

func readConf() {
	var err os.Error
	conf, err = config.ReadDefault("rbot.conf")
	if (err != nil) {
		fmt.Printf("Config error: %s\n", err)
		os.Exit(1)
	}
}
func readConfString(section, option string) string {
	value, err := conf.String(section, option)
	if err != nil {
		panic(fmt.Sprintf("Config error: %s", err))
	}
	return value
}
func readConfBool(section, option string) bool {
	value, err := conf.Bool(section, option)
	if err != nil {
		panic(fmt.Sprintf("Config error: %s", err))
	}
	return value
}
func updateConf(section, option, value string) {
	conf.AddOption(section, option, value)
	if err := conf.WriteFile(confFile, 0644, ""); err != nil {
		panic("Error while writing to " + confFile)
	}
}

func identd(port string) {
	identd, err := net.Listen("tcp", "0.0.0.0:" + port)
	if err != nil {
		fmt.Println("Failed to start identd on port", port)
		return
	}
	defer identd.Close()
	fmt.Println("Started identd on port", port)

	for {
		conn, err := identd.Accept()
		if err != nil {
			fmt.Println("Failed to accept identd connection")
			continue
		}

		io := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		line, err := io.Reader.ReadString('\n')
		if err != nil || len(line) < 2 {
			conn.Close()
			fmt.Println("Failed to read identd request")
			continue
		}
		line = line[:len(line) - 2]
		line = fmt.Sprintf("%s : ERROR : NO-USER\r\n", line)
		io.Writer.WriteString(line)
		time.Sleep(1000000000) // 1 second
		conn.Close()
	}
}
