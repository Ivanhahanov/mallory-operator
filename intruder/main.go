package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const banner = `
  _____       _                  _           
 |_   _|     | |                | |          
   | |  _ __ | |_ _ __ _   _  __| | ___ _ __ 
   | | | '_ \| __| '__| | | |/ _  |/ _ \ '__|
  _| |_| | | | |_| |  | |_| | (_| |  __/ |   
 |_____|_| |_|\__|_|   \__,_|\__,_|\___|_|   
                        by mallory-operator
`

type Broadcaster struct {
	mu          sync.Mutex
	subscribers []chan string
	history     []string
}

// Subscribe creates a new channel for subscription,
// sends all accumulated history to it and adds it to the subscribers list.
func (b *Broadcaster) Subscribe() chan string {
	ch := make(chan string, 100)
	b.mu.Lock()
	// Отправляем историю новому подписчику.
	for _, msg := range b.history {
		ch <- msg
	}
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes the channel from the subscribers list and closes it.
func (b *Broadcaster) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, subscriber := range b.subscribers {
		if subscriber == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

// Broadcast saves the message in history and sends it to all subscribers.
func (b *Broadcaster) Broadcast(msg string) {
	b.mu.Lock()
	b.history = append(b.history, msg)
	for _, subscriber := range b.subscribers {
		// Отправляем сообщение без блокировки.
		select {
		case subscriber <- msg:
		default:
		}
	}
	b.mu.Unlock()
}

var broadcaster = &Broadcaster{}

func main() {
	// Флаги: -cmd для передачи bash-команды и -port для указания порта
	cmdFlag := flag.String("c", "", "command for execute (example: \"ping -c 5 google.com\")")
	portFlag := flag.String("port", "8080", "webserver port")
	shellFlag := flag.String("shell", "sh", "shell for execute commands")
	flag.Parse()

	if *cmdFlag == "" {
		log.Fatal("Пожалуйста, передайте команду через флаг -cmd")
	}

	// Run the command execution in a separate goroutine.
	go runCommand(*shellFlag, *cmdFlag)

	// Configure HTTP handlers.
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/stream", streamHandler)
	fmt.Println(banner)
	log.Printf("server starts on :%s", *portFlag)
	if err := http.ListenAndServe(":"+*portFlag, nil); err != nil {
		log.Fatal(err)
	}
}

// runCommand executes the given shell command and translates the output line by line.
func runCommand(shell string, cmdStr string) {
	log.Printf("input command: %s", cmdStr)
	broadcaster.Broadcast(fmt.Sprintf("> %s -c \"%s\"", shell, cmdStr))

	cmd := exec.Command(shell, "-c", cmdStr)

	// Получаем stdout, stderr объединяем со stdout.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Ошибка получения stdout: %v", err)
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		log.Printf("Ошибка запуска команды: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		broadcaster.Broadcast(line)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading command output: %v", err)
	}

	err = cmd.Wait()
	if err != nil {
		broadcaster.Broadcast(fmt.Sprintf("The command terminated with an error: %v", err))
	} else {
		broadcaster.Broadcast("Command completed successfully!")
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	var stats string
	kubeAPIServer := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubeAPIServerPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	hostname := os.Getenv("HOSTNAME")

	if kubeAPIServer != "" {
		namespace := "N/A"
		if nsData, err := os.ReadFile("/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = strings.TrimSpace(string(nsData))
		}

		serviceToken := "N/A"
		if tokenData, err := os.ReadFile("/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
			serviceToken = strings.TrimSpace(string(tokenData))
		}
		stats = fmt.Sprintf(`
    <h3>Running in Kubernetes</h3>
    <ul>
      <li><strong>API-сервер:</strong> %s:%s</li>
      <li><strong>Namespace:</strong> %s</li>
      <li><strong>Hostname:</strong> %s</li>
	  <li>
	  	<strong>Service Account Token:</strong> 
		<button onclick="document.getElementById('token').classList.toggle('hidden')">Show</button>
		<pre id="token" class="hidden">%s</pre>
	  </li>
	  <style>
		.hidden { display: none; }
	  </style>
    </ul>
	`, kubeAPIServer, kubeAPIServerPort, namespace, hostname, serviceToken)
	} else {
		stats = "<h3>Running outside of kubernetes</h3>"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Intruder</title>
  <style>
    body { 
      font-family: sans-serif; 
      padding: 2rem; 
      background-color: #121212;
      color: #ffffff;
    }
    pre { 
      background: #1e1e1e; 
      padding: 1rem; 
      border: 1px solid #444;
      color: #ffffff;
	  word-wrap: break-word;
	  white-space: pre-wrap;
	  overflow-wrap: break-word;
	  max-width: 100%%;
    }
    ul { 
      list-style: none; 
      padding: 0; 
    }
    li { 
      margin-bottom: 0.5rem; 
    }
    .banner {
      font-family: monospace;
      white-space: pre;
      background: #1e1e1e;
      padding: 1rem;
      border: 1px solid #444;
      margin-bottom: 1rem;
    }
  </style>
</head>
<body>
  <div class="banner">%s</div>
  %s
  <h2>Log</h2>
  <pre id="log"></pre>
  <script>
    const logElement = document.getElementById('log');
    const evtSource = new EventSource("/stream");
    evtSource.onmessage = function(e) {
      logElement.textContent += e.data + "\n";
    };
  </script>
</body>
</html>`, banner, stats)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := broadcaster.Subscribe()
	defer broadcaster.Unsubscribe(ch)

	notify := r.Context().Done()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-notify:
			return
		}
	}
}
