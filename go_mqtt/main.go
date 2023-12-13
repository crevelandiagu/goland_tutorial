package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type frame struct {
	FROM    string `jsom:from`
	TO      string `json:to`
	MESSAGE string `json:message`
	key     string //lowercase:it means it won't be marshalled
	fan     string //lowercase:it means it won't be marshalled
}

func (s frame) jsonMaker() []byte {
	jsonData, err := json.Marshal(s)
	if err != nil {
		log.Fatal("Error encoding JSON:", err)
	}
	return jsonData

}

func structMaker(jsonData []byte) frame {
	var receive frame
	err := json.Unmarshal(jsonData, &receive)
	err = err
	//check4errors("unmarshalling", err)
	return receive

}

var wg sync.WaitGroup

func main() {

	var send frame

	//set hostname of rabbitMQ service to connect to
	var hostname string = "192.168.51.100"
	var port string = "5672"
	if len(os.Args) > 1 {
		flag.StringVar(&hostname, "host", "192.168.51.100", "--host hostname")
		flag.StringVar(&port, "port", "5672", "--port 5672")
		flag.Parse()
	}

	uri := "amqp://guest:guest@" + hostname + ":" + port + "/"
	fmt.Println(uri)
	conn, err := amqp.Dial(uri)
	check4errors("Logging to RabbitMQ", err)
	defer conn.Close()

	//make a channel once successfully connected to rabbitMQ
	ch, err := conn.Channel()
	check4errors("Connecting to a channel:", err)
	defer ch.Close()

	//provide nick or name to identify your communication queue
	scanner := bufio.NewScanner(os.Stdin)
provideYourNick:
	fmt.Print("provide a nick: ")
	scanner.Scan()
	username := scanner.Text()
	if checkIfAllowed(username) {
		goto provideYourNick
	}
	fmt.Printf("username:%s\n", username)
	send.FROM = username

	//declare exclusive queue accessible only for single consumer of queued information
	q, err := ch.QueueDeclare(
		username, // unikalna nazwa kolejki dla użytkownika
		false,    // durable
		false,    // delete when unused
		true,     // exclusive
		false,    // no-wait
		nil,      // arguments
	)
	check4errors("Exclusive queue declaration for "+username, err)

	//declare exchange to bind with exclusive queue for ALL broadcast messages to all participants of lanChat
	err = ch.ExchangeDeclare(
		"ALL",    // name of an exchange
		"fanout", // typ exchange
		false,    // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	check4errors("Exchange creation:", err)

	//binding Exchange with exclusive q
	err = ch.QueueBind(
		q.Name, // queue name
		"",     // routing key (puste dla fanout)
		"ALL",  // nazwa fanout exchange
		false,  // no-wait
		nil,    // arguments
	)
	check4errors("Bind "+q.Name+"'s queue to the exchange", err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			send.TO = "ALL" //default
			send.fan = "ALL"
			send.key = ""
			scanner.Scan()
			cmessage := scanner.Text()
			commands, message := parseMessage(cmessage)
			send.MESSAGE = message
			err := send.executeListOf(commands)
			err = err // comment this in case of uncommenting the line below
			//check4errors("commands execution", err)
			jsonData := send.jsonMaker()
			//fmt.Println("json debug", string(jsonData))
			send.sendMessage(jsonData, ch)
			//check4errors("broadcasting", err)// 4 debug only
		}
	}()

	msgs, err := ch.Consume(
		q.Name, // queue name
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	//check4errors("consume from queue:"+q.Name, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgs {

			receive := structMaker(msg.Body)
			fmt.Printf("\033[7m %s->%s \033[0m %s\n", receive.FROM, receive.TO, receive.MESSAGE)
		}
	}()
	wg.Wait()
}

func check4errors(s string, err error) {
	if err != nil {
		log.Println(s, "\x1b[1;31mError:", err)
		os.Exit(-1)
	} else {
		log.Println(s, "\x1b[1;92m"+"Successful\x1b[m")
	}
}

func (s *frame) sendMessage(jsonData []byte, ch *amqp.Channel) {
	if len(s.MESSAGE) > 0 {
		err := ch.Publish(
			s.fan, // nazwa fanout exchange
			s.key, // routing key (puste dla fanout)
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        jsonData,
			},
		)
		err = err
	}
	//check4errors("sendMessage", err) //4 debug only
}

// parseMessage make slice of commands add cut away the commands from the message leaving msg to be redy do send or receive.
func parseMessage(s string) ([]string, string) {
	var control []string
	//var cwords []string
	var words []string
	var sentence string

	cwords := strings.Fields(s) //break message by the words with commands
	for _, word := range cwords {
		if strings.HasPrefix(word, "@") { //if word is command append it to control
			control = append(control, word)
		} else {
			words = append(words, word)
		}
	}
	sentence = strings.Join(words, " ")
	return control, sentence
}

func (s *frame) executeListOf(commands []string) error {

	for _, virt := range commands {

		switch virt {
		case "@exit":
			os.Exit(0)
		case "@help":
			help()
			s.MESSAGE = ""
		case "@ALL":
			s.TO = "ALL"

		default:
			s.TO = strings.TrimPrefix(virt, "@")
			s.key = s.TO
			s.fan = ""
			//fmt.Println("debug:s.TO", s.TO)
		}

	}
	return nil
}

func help() {
	fmt.Println("\033[A\r@help show this info\n@exit to quit the chat\n@users to list all participants\n@name to send private message for certain user")
}

func checkIfAllowed(username string) bool {
	if username == "ALL" || username == "help" || username == "exit" || username == "users" {
		return true
	} else {
		return false
	}
}
