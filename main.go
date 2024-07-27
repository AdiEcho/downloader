package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	f, err := os.OpenFile("downloader.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, os.ModePerm)
	if err != nil {
		log.Fatalln("open log file error:", err)
		return
	}
	defer f.Close()
	log.SetOutput(f)

	var urls []string
	var urlsStr string
	versionFlag := flag.Bool("version", false, "print the version")
	flag.StringVar(&urlsStr, "u", "", "urls to download, use ',' to split")
	flag.IntVar(&numThreads, "t", 4, "number of threads for each file download")
	flag.IntVar(&maxRetries, "max_retries", 5, "max retries for download")
	flag.BoolVar(&useUrlFilename, "use_url_filename", true, "use filename from url or generate downloaded_0, downloaded_1, ...")
	flag.DurationVar(&retryDelay, "retry_delay", time.Second*2, "delay between retries")
	flag.Parse()

	switch {
	case *versionFlag:
		fmt.Printf(Version)
		return
	case urlsStr != "":
		urls = strings.Split(urlsStr, ",")
		DownloadFiles(urls)
	default:
		flag.Usage()
		return
	}
}
