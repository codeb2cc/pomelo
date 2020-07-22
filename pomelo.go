package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/crc32"
	"index/suffixarray"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"golang.org/x/text/unicode/norm"
)

const (
	maxLookup = 2048
	delimiter = '\x00'
	stdinFlag = "@stdin"
)

var (
	saIndexes map[string]*suffixarray.Index
	logger    *log.Logger
)

type Item struct {
	Query string
	Value uint64
}

func buildIndex(src, dst string, length, value uint) (int, error) {
	var srcFile *os.File
	var err error
	if src == stdinFlag {
		srcFile = os.Stdin
	} else {
		srcFile, err = os.Open(src)
		if err != nil {
			return 0, err
		}
		defer srcFile.Close()
	}

	var buffer bytes.Buffer
	var val uint64
	valBuf := make([]byte, binary.MaxVarintLen64)
	counter := 0
	scanner := bufio.NewScanner(srcFile)
	for scanner.Scan() {
		parts := bytes.SplitN(scanner.Bytes(), []byte("\t"), 2)
		if len(parts) == 1 { // 无权数据
			val = 0
		} else if len(parts) == 2 { // 带权数据
			val, err = strconv.ParseUint(string(parts[1]), 10, 32)
			if err != nil || val < uint64(value) {
				continue
			}
		} else { // 数据格式错误
			continue
		}
		if uint(len(parts[0])) > length {
			continue
		}
		counter += 1

		binary.PutUvarint(valBuf, val)
		buffer.Write(norm.NFC.Bytes(bytes.Trim(parts[0], string(delimiter))))
		buffer.Write(valBuf)
		buffer.Write([]byte(string(delimiter)))
	}
	saIndex := suffixarray.New(buffer.Bytes())

	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer dstFile.Close()

	return counter, saIndex.Write(dstFile)
}

func loadIndex(src, key string) (string, error) {
	file, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if key == "" {
		h := crc32.NewIEEE()
		io.Copy(h, file)
		key = fmt.Sprintf("%x", h.Sum32())
	}
	saIndexes[key] = suffixarray.New([]byte{})

	file.Seek(0, 0)
	return key, saIndexes[key].Read(file)
}

func indexLookup(index *suffixarray.Index, search string) []Item {
	offsets := index.Lookup([]byte(search), maxLookup)

	items := []Item{}
	indexData := index.Bytes()
	for _, offset := range offsets {
		start, end := offset, offset
		for ; start > 0 && indexData[start-1] != delimiter; start-- {
		}
		for ; end < len(indexData) && indexData[end] != delimiter; end++ {
		}
		for ; end+1 < len(indexData) && indexData[end+1] == delimiter; end++ { // consume the rest delimiter bytes
		}
		if end-offset < binary.MaxVarintLen64 { // indexed bytes in the value segment
			continue
		}
		val, _ := binary.Uvarint(indexData[end-binary.MaxVarintLen64 : end])
		item := Item{
			string(indexData[start : end-binary.MaxVarintLen64]),
			val,
		}
		items = append(items, item)
	}

	return items
}

func startConsole(index *suffixarray.Index) {
	var searchStr string
	for {
		fmt.Printf(">> Search for: ")
		if n, _ := fmt.Scanln(&searchStr); n == 0 {
			return
		}

		t0 := time.Now()
		items := indexLookup(index, searchStr)
		t1 := time.Now()

		fmt.Printf(">>   %v records found in %v:\n", len(items), t1.Sub(t0))
		for _, item := range items {
			fmt.Printf("%v\t%v\n", item.Query, item.Value)
		}
		fmt.Println()
	}
}

func usage() {
	fmt.Printf(`Usage: pomelo COMMAND [OPTIONS]

Command:
	-console -index=PATH
	-web [-index=PATH] [-http=:8080] [-procs=2]
	-build -src=PATH -dst=PATH [-max-length=120] [-min-value=1000]
`)
}

func main() {
	logger = log.New(os.Stdout, "[Pomelo] ", log.Ltime|log.Lshortfile)

	var cmdConsole, cmdWeb, cmdBuild bool
	flag.BoolVar(&cmdConsole, "console", false, "")
	flag.BoolVar(&cmdWeb, "web", false, "")
	flag.BoolVar(&cmdBuild, "build", false, "")

	var indexData, indexKey, httpAddr string
	var procs int
	flag.StringVar(&indexData, "index", "", "index data path")
	flag.StringVar(&indexKey, "key", "", "index key")
	flag.StringVar(&httpAddr, "http", ":8080", "web server address")
	flag.IntVar(&procs, "procs", 2, "max process number")

	var src, dst string
	var maxLength, minValue uint
	flag.StringVar(&src, "src", "", "input data file path")
	flag.StringVar(&dst, "dst", "", "output index data file path")
	flag.UintVar(&maxLength, "max-length", 120, "max length of index entry")
	flag.UintVar(&minValue, "min-value", 1000, "minimum value of index entry")

	flag.Usage = func() { usage() }
	flag.Parse()

	var err error
	if cmdConsole || cmdWeb {
		saIndexes = make(map[string]*suffixarray.Index)
		if indexData != "" {
			indexKey, err = loadIndex(indexData, indexKey)
			if err != nil {
				fmt.Printf("Load index data from %v failed.\n", indexData)
				os.Exit(1)
			}
		} else if cmdConsole {
			usage()
			os.Exit(1)
		}

		if cmdConsole {
			startConsole(saIndexes[indexKey])
		} else if cmdWeb {
			runtime.GOMAXPROCS(procs)
			fmt.Printf(">> Running index service on %v ...\n", httpAddr)
			if err := startWebServer(httpAddr); err != nil {
				fmt.Println(err)
			}
		}
	} else if cmdBuild {
		if src == "" && dst == "" {
			usage()
			os.Exit(1)
		}

		t0 := time.Now()
		count, err := buildIndex(src, dst, maxLength, minValue)
		if err != nil {
			fmt.Printf("Unknown error: %v", err)
			os.Exit(1)
		}
		t1 := time.Now()
		fmt.Printf("Succeed to build index for %v entries in %v:\nDone.\n", count, t1.Sub(t0))
	} else {
		usage()
		os.Exit(1)
	}

	fmt.Printf("Bye-Bye!\n")
}
