package garchive

import (
	"fmt"
	"github.com/gerow/girc"
	"log"
	"os"
	"strings"
	"time"
)

func MakeChannelListener(connection *girc.Connection, channel string, filename string) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	listener_channel := make(chan *girc.Command, 10)
	connection.AddListener(listener_channel)

	// start a routine to handle them
	go func() {
		for {
			command, ok := <-listener_channel
			if !ok {
				break
			}
			line := ""
			switch command.Type {
			case "PRIVMSG":
				if command.Args[0] == channel {
					line = fmt.Sprintf("%s: %s\n", command.Source, command.Args[1])
				}
			case "JOIN":
				if command.Args[0] == channel {
					line = fmt.Sprintf("%s has joined %s\n", command.Source, channel)
				}
			case "PART":
				if command.Args[0] == channel {
					line = fmt.Sprintf("%s has left %s\n", command.Source, channel)
				}
			case "TOPIC":
				if command.Args[0] == channel {
					line = fmt.Sprintf("%s has set the topic to %s\n", command.Source, command.Args[1])
				}
			case "QUIT":
				line = fmt.Sprintf("%s has quit: %s\n", command.Source, command.Args[0])
			}

			if line != "" {
				fmt.Fprintf(file, "[%v] %s", time.Now(), line)
			}
		}
	}()

	return nil
}

func Main() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: %s irc_uri nick channel [other_channels...]\n", os.Args[0])
		os.Exit(1)
	}
	uri := os.Args[1]
	nick := os.Args[2]
	channels := os.Args[3:len(os.Args)]
	connection := girc.New(uri, nick)

	err := connection.Connect()
	if err != nil {
		log.Fatal(err)
	}

	for _, c := range channels {
		/* allow the user to specify the channel without the #
		 * since bash requries it to be escaped */
		if !strings.HasPrefix(c, "#") {
			c = fmt.Sprintf("#%s", c)
		}
		err = connection.Send("JOIN", c)
		if err != nil {
			log.Print(err)
		}
		err := MakeChannelListener(connection, c, strings.TrimPrefix(c, "#"))
		if err != nil {
			log.Print(err)
		}
	}

	<-connection.Finished
}
