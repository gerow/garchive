package garchive

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
        "time"
)

type IRCConnection struct {
	Location   string
	Nick       string
	Connection net.Conn
	Listeners  []func(*IRCCommand)
	Finished   chan bool
}

type IRCCommand struct {
	Source string
	Type   string
	Args   []string
}

type IRCListener func(*IRCCommand)

func MakeConnection(uri string, nick string) *IRCConnection {
	var connection IRCConnection

	connection.Location = uri
	connection.Nick = nick

	return &connection
}

/* go from a raw string form of a command to an IRCCommand */
func RawToIRCCommand(raw string) (*IRCCommand, error) {
	var command IRCCommand

	split_ver := strings.Split(raw, " ")
	/* first as a sanity check make sure that our array has at least
	   two entries, any less is not a valid command */
	if len(split_ver) < 2 {
		return &command, errors.New("invalid command (less than two entries in command)")
	}
	args_start := 2
	if strings.HasPrefix(split_ver[0], ":") {
                command.Source = strings.TrimPrefix(split_ver[0], ":")
		command.Type = split_ver[1]
	} else {
		command.Type = split_ver[0]
		args_start = 1
	}

	/* iterate over every element after the first two */
	multi_word_index := -1
	for index, arg := range split_ver[args_start:] {
		if strings.HasPrefix(arg, ":") {
			multi_word_index = index
			break
		}

		command.Args = append(command.Args, arg)
	}

	if multi_word_index != -1 {
		words := []string{}
		words = append(words, split_ver[args_start:][multi_word_index][1:len(split_ver[args_start:][multi_word_index])])
		words = append(words, split_ver[args_start:][multi_word_index+1:]...)
		command.Args = append(command.Args, strings.Join(words, " "))
	}

	command.Args[len(command.Args)-1] = strings.TrimSuffix(command.Args[len(command.Args)-1], "\r\n")

	return &command, nil
}

func MakeIRCCommand(cmdtype string, args ...string) *IRCCommand {
	var command IRCCommand

	command.Type = cmdtype
	command.Args = args

	return &command
}

func (command *IRCCommand) ToRaw() (string, error) {
	out := []string{}
	if command.Source != "" {
		out = append(out, command.Source)
	}
	out = append(out, command.Type)
	for _, arg := range command.Args[0 : len(command.Args)-1] {
		if strings.Contains(arg, " ") {
			return "", errors.New("nonfinal argument contains space")
		}
		out = append(out, arg)
	}

	if strings.Contains(command.Args[len(command.Args)-1], " ") {
		out = append(out, fmt.Sprint(":", command.Args[len(command.Args)-1]))
	} else {
		out = append(out, command.Args[len(command.Args)-1])
	}

	return fmt.Sprintf("%s\r\n", strings.Join(out, " ")), nil
}

func (connection *IRCConnection) Connect() error {
	log.Print("Connecting...")
	conn, err := net.Dial("tcp", connection.Location)
	connection.Connection = conn

	if err != nil {
		return err
	}

	log.Print("Connected!")

	go func() {
		//bio := bufio.NewReader(conn)
		for {
			line, err := bufio.NewReader(conn).ReadString('\n')
			log.Printf("Got line: %s\n", line)
			if err != nil {
				connection.Finished <- true
				log.Fatal(err)
			}
			command, err := RawToIRCCommand(line)
			if err != nil {
				log.Print(err)
			}
			for _, fn := range connection.Listeners {
				go fn(command)
			}
		}
	}()

	/* create a goroutine to send PONGs back when we receive PINGs */
	connection.AddListener(func(command *IRCCommand) {
		if command.Type == "PING" {
			fmt.Fprintf(conn, "PONG %s\r\n", command.Args[0])
		}
	})

        err = connection.SendCommand(MakeIRCCommand("NICK", "garchive"))
        if err != nil {
          return err
        }
        err = connection.SendCommand(MakeIRCCommand("USER", "guest", "hostname", "what", "My Real Name"))
        if err != nil {
          return err
        }

	return nil
}

func (connection *IRCConnection) AddListener(fn IRCListener) {
	connection.Listeners = append(connection.Listeners, fn)
}

func (connection *IRCConnection) SendCommand(command *IRCCommand) error {
  log.Printf("Sending command %v\n", command)
  raw_form, err := command.ToRaw()
  log.Printf("raw form: %v\n", raw_form)
  if err != nil {
    return err
  }

  fmt.Fprint(connection.Connection, raw_form)

  return nil
}

func MakeChannelListener(channel string, filename string) (func(*IRCCommand), error) {
  file, err := os.OpenFile(filename, os.O_WRONLY | os.O_APPEND | os.O_CREATE, 0666 )
  if err != nil {
    return nil, err
  }

  fn := func(command *IRCCommand) {
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
    }

    if line != "" {
      fmt.Fprintf(file, "[%v] %s", time.Now(), line)
    }
  }

  return fn, nil
}

func Main() {
	connection := MakeConnection("irc.tropicalmug.com:6667", "garchive")

	connection.AddListener(func(command *IRCCommand) {
		fmt.Printf("%v\n", command)
		fmt.Printf("Args:\n")
		for index, arg := range command.Args {
			fmt.Printf("%d.  %s\n", index, arg)
		}
	})

	err := connection.Connect()
	if err != nil {
		log.Fatal(err)
	}

        err = connection.SendCommand(MakeIRCCommand("JOIN", "#chat"))
        if err != nil {
          log.Fatal(err)
        }
        channelListener, err := MakeChannelListener("#chat", "/tmp/chat")
        if err != nil {
          log.Fatal(err)
        }
        connection.AddListener(channelListener)

        <-connection.Finished

	/*
		conn, err := net.Dial("tcp", "irc.tropicalmug.com:6667")
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			i := 0
			for {
				status, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("msg %d: %s", i, status)
				i++
			}
		}()
		fmt.Fprintf(conn, "NICK garchive\r\n")
		fmt.Fprintf(conn, "USER guest hostname what :My Real Name\r\n")
		bio := bufio.NewReader(os.Stdin)
		for {
			line, _, err := bio.ReadLine()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Fprintf(conn, "%s\r\n", line)
		}
	*/
}
