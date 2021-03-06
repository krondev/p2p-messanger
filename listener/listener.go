package listener

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/easmith/p2p-messanger/types"
	"io"
	"io/ioutil"

	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func StartListener(port int, ch chan string, peers *types.Peers) {
	service := fmt.Sprintf(":%v", port)

	tcpAddr, err := net.ResolveTCPAddr("tcp", service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ResolveTCPAddr: %s", err.Error())
		os.Exit(1)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListenTCP: %s", err.Error())
		os.Exit(1)
	}

	log.Println("Start listen " + service)
	for {
		conn, err := listener.Accept()
		if err != nil {
			ch <- "conn Accept error: " + err.Error()
			continue
		}
		// TODO: общение через канал
		ch <- "new connection: " + conn.RemoteAddr().String()
		go onConnection(conn, peers)
	}

	ch <- "done"
}

func onConnection(conn net.Conn, peers *types.Peers) {
	defer func() {
		peers.Remove(conn)
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	readWriter := bufio.NewReadWriter(reader, writer)

	buf, err := readWriter.Peek(4)
	log.Printf("Start read peak")
	if err != nil {
		log.Printf("Read peak ERROR: %s", err)
		return
	}
	log.Printf("Stop read peak: %v", string(buf))

	if types.ItIsMessanger(buf) {
		log.Println("This is message!")
		handlePeer(readWriter, conn, peers)
	} else {
		log.Println("Try request")
		handleRequest(readWriter, conn)
	}
}

func handlePeer(rw *bufio.ReadWriter, conn net.Conn, peers *types.Peers) {
	var buf = make([]byte, 1024)
	var peer *types.Peer
	for {
		var cmd = make([]byte, 4)
		n, err := rw.Read(cmd)
		if err != nil {
			if err == io.EOF {
				log.Printf("Disconected by EOF")
			} else {
				log.Printf("handlePeer ERROR: %s", err)
			}
			return
		}

		log.Printf("MAIN Recieved: [%v] %s", n, bytes.Trim(cmd, "\r\n\x00"))

		switch string(cmd) {
		case "NAME":
			{
				line, _, _ := rw.ReadLine()
				id := types.Id(bytes.Trim(line, " "))
				_, found := peers.ById.Get(id)
				if found {
					conn.Write([]byte("ERR Name already in use\n"))
					continue
				}
				peer = peers.Add(&conn, id)
				log.Printf("new peer: %s", peer)
				conn.Write([]byte("OK\n"))
			}
		case "LIST":
			{
				rw.Read(buf)
				for _, p := range peers.ById.Peers {
					conn.Write([]byte(fmt.Sprintf("PEER\t%v\t%s\n", p.Addr, p.Id)))
				}

			}
		case "SEND":
			{
				if peer == nil {
					//conn.Write([]byte(fmt.Sprintf("ERR you not registered (use name)\n")))
					continue
				}

				to, _ := rw.ReadString(0x20)

				p, found := peers.ById.Get(types.Id(to[0 : len(to)-1]))
				if !found {
					conn.Write([]byte(fmt.Sprintf("ERR not found %v\n", types.Id(to))))
					continue
				}

				msg, err := rw.ReadString('\n')
				if err != nil {
					log.Printf("ReadMessageError: %s", err)
					continue
				}

				_, err = (*p.Conn).Write([]byte(fmt.Sprintf("MESS %v: %v", peer.Id, msg)))
				if err != nil {
					log.Printf("WriteMessageError: %s", err)
					(*peer.Conn).Write([]byte("ERR " + err.Error()))
					continue
				}

				(*peer.Conn).Write([]byte("OK\n"))
			}
		default:
			conn.Write([]byte("UNKNOWN_CMD\n"))
		}
	}
}

func handleRequest(rw *bufio.ReadWriter, conn net.Conn) {
	request, err := http.ReadRequest(rw.Reader)

	if err != nil {
		log.Printf("Read request ERROR: %s", err)
		return
	}

	response := http.Response{
		StatusCode: 200,
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	s := conn.RemoteAddr().String()[0:3] + "REMOVEIT"
	// TODO: сравнение среза со строкой
	if strings.EqualFold(s, "127") || strings.EqualFold(s, "[::") {
		response.Body = ioutil.NopCloser(strings.NewReader("php-messenger 1.0"))
	} else {

		if path.Clean(request.URL.Path) == "/ws" {
			handleWs(NewWriter(conn), request)
			return
		} else {
			processRequest(request, &response)
		}
	}

	err = response.Write(rw)
	if err != nil {
		log.Printf("Write response ERROR: %s", err)
		return
	}

	err = rw.Writer.Flush()
	if err != nil {
		log.Printf("Flush response ERROR: %s", err)
		return
	}
}

func processRequest(request *http.Request, response *http.Response) {
	path := path.Clean(request.URL.Path)

	log.Printf("Request: %v\n", path)

	filePath := "./front/build" + path

	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		response.StatusCode = 404
		response.Body = ioutil.NopCloser(strings.NewReader("Not found!"))
		return
	}

	if info.IsDir() {
		_, err := os.Stat(filePath + "index.html")
		if err == nil {
			responseFile(response, filePath+"index.html")
			return
		}

		files, err := readDir(filePath)
		if err != nil {
			response.StatusCode = 500
			// TODO: приведение ошибки к нужному типу, для получения конкретных свойств
			response.Body = ioutil.NopCloser(strings.NewReader("Internal server error: " + err.(*os.PathError).Err.Error()))
		}
		filesString := strings.Join(files[:], "\n")
		response.Body = ioutil.NopCloser(strings.NewReader("Index of " + path + ":\n\n" + filesString))
		return
	}

	responseFile(response, filePath)
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, w.Header())
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Printf("ws read error: %v", err)
			break
		}
		log.Printf("ws read: [%v] %s", mt, message)
		err = c.WriteMessage(mt, append([]byte("server recv: "), message...))
		if err != nil {
			log.Printf("ws write error: %s", err)
			break
		}
	}
}

func responseFile(response *http.Response, fileName string) {
	file, err := os.Open(fileName)

	if os.IsPermission(err) {
		response.StatusCode = 403
		response.Body = ioutil.NopCloser(strings.NewReader("Forbidden"))
		return
	} else if err != nil {
		response.StatusCode = 500
		response.Body = ioutil.NopCloser(strings.NewReader("Internal server error: " + err.(*os.PathError).Err.Error()))
		return
	}

	response.Body = file
}

func readDir(root string) ([]string, error) {
	var files []string
	f, err := os.Open(root)
	if err != nil {
		return files, err
	}
	fileInfo, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return files, err
	}

	for _, file := range fileInfo {
		// TODO: нет тернарного оператора
		// files = append(files, file.Name() + (file.IsDir() ? "/" : ""))
		if file.IsDir() {
			files = append(files, file.Name()+"/")
		} else {
			files = append(files, file.Name())
		}
	}
	return files, nil
}

type MyWriter struct {
	conn net.Conn
}

func (w MyWriter) Write(b []byte) (int, error) {
	return w.conn.Write(b)
}

func (w MyWriter) Header() http.Header {
	return http.Header{}
}

func (w MyWriter) WriteHeader(statusCode int) {
	_, err := w.conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK")))
	if err != nil {
		log.Printf("WriteHeaderError: %v\n", err)
	}
}

func (w MyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	reader := bufio.NewReader(w.conn)
	writer := bufio.NewWriter(w.conn)

	readWriter := bufio.NewReadWriter(reader, writer)
	return w.conn, readWriter, nil
}

func NewWriter(conn net.Conn) http.ResponseWriter {
	return &MyWriter{conn}
}

//
