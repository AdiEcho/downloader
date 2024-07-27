package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DownloadPart struct {
	Index  int
	Start  int64
	End    int64
	Buffer []byte
}

var (
	numThreads     = 4
	maxRetries     = 5
	retryDelay     = time.Second * 2
	useUrlFilename = true
	Version        = "1.0.0"
)

const progressBarWidth = 50

type FileProgress struct {
	Filename     string
	Downloaded   int64
	TotalSize    int64
	CurrentSpeed int64
}

func DownloadFiles(urls []string) {
	var wg sync.WaitGroup
	progressChans := make([]chan FileProgress, len(urls))
	progressStates := make([]FileProgress, len(urls))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i, url := range urls {
		progressChans[i] = make(chan FileProgress)
		wg.Add(1)
		var filename string
		if useUrlFilename {
			filename = parseFilename(url)
		} else {
			filename = "downloaded_" + strconv.Itoa(i)
		}
		go func(i int, url string) {
			defer wg.Done()
			DownloadFile(ctx, url, filename, progressChans[i])
		}(i, url)
	}

	// 启动进度显示器
	go displayProgress(ctx, progressChans, progressStates)

	wg.Wait()
	for _, ch := range progressChans {
		close(ch)
	}
	fmt.Println("\nAll files downloaded.")
}

func DownloadFile(ctx context.Context, url, output string, progressChan chan<- FileProgress) {
	resp, err := http.Head(url)
	if err != nil {
		log.Printf("Error getting file size: %v", err)
		progressChan <- FileProgress{Filename: output, Downloaded: -1}
		return
	}
	defer resp.Body.Close()

	size := resp.ContentLength

	parts := make([]DownloadPart, numThreads)
	partSize := size / int64(numThreads)

	for i := 0; i < numThreads; i++ {
		start := int64(i) * partSize
		end := start + partSize - 1
		if i == numThreads-1 {
			end = size - 1
		}

		parts[i] = DownloadPart{
			Index:  i,
			Start:  start,
			End:    end,
			Buffer: make([]byte, end-start+1),
		}
	}

	var wg sync.WaitGroup
	progress := make(chan int64, numThreads)

	for _, part := range parts {
		wg.Add(1)
		go func(part DownloadPart) {
			defer wg.Done()
			downloadPartWithRetry(ctx, url, &part, progress)
		}(part)
	}

	go monitorProgress(ctx, progress, size, output, progressChan)

	wg.Wait()
	close(progress)

	saveFile(output, parts)
	progressChan <- FileProgress{
		Filename:   output,
		Downloaded: size,
		TotalSize:  size,
	}
}

func downloadPartWithRetry(ctx context.Context, url string, part *DownloadPart, progress chan<- int64) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := downloadPart(ctx, url, part, progress)
		if err == nil {
			return
		}
		log.Printf("Error downloading part %d, attempt %d: %v. Retrying...\n", part.Index, attempt, err)
		select {
		case <-ctx.Done():
			log.Printf("Cancelled downloading part %d\n", part.Index)
			return
		case <-time.After(retryDelay):
		}
	}
	log.Printf("Failed to download part %d after %d attempts\n", part.Index, maxRetries)
}

func downloadPart(ctx context.Context, url string, part *DownloadPart, progress chan<- int64) error {
	client := &http.Client{}
	req, err := createRequest(ctx, url, part.Start, part.End)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error downloading part: %w", err)
	}
	defer resp.Body.Close()

	return readResponseBody(resp.Body, part, progress)
}

func createRequest(ctx context.Context, url string, start, end int64) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	return req, nil
}

func readResponseBody(body io.Reader, part *DownloadPart, progress chan<- int64) error {
	buffer := make([]byte, 1024)
	var totalDownloaded int64

	for {
		n, err := body.Read(buffer)
		if n > 0 {
			copy(part.Buffer[totalDownloaded:], buffer[:n])
			totalDownloaded += int64(n)
			progress <- int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
	}

	return nil
}

func saveFile(filename string, parts []DownloadPart) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	defer file.Close()

	for _, part := range parts {
		_, err := file.Write(part.Buffer)
		if err != nil {
			log.Printf("Error writing to file: %v", err)
			return
		}
	}
}

func formatSpeed(speed int64) string {
	if speed > 1024*1024 {
		return fmt.Sprintf("%.2f MB/s", float64(speed)/1024/1024)
	} else if speed > 1024 {
		return fmt.Sprintf("%.2f KB/s", float64(speed)/1024)
	} else {
		return fmt.Sprintf("%d B/s", speed)
	}
}

func formatSize(size int64) string {
	if size > 1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(size)/1024/1024/1024)
	} else if size > 1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(size)/1024/1024)
	} else if size > 1024 {
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else {
		return fmt.Sprintf("%d B", size)
	}
}

func getProgressBar(progress FileProgress) string {
	downloaded := progress.Downloaded
	total := progress.TotalSize
	speed := formatSpeed(progress.CurrentSpeed)

	var progressRatio float64
	if total > 0 {
		progressRatio = float64(downloaded) / float64(total)
	} else {
		progressRatio = 0
	}
	barWidth := int(progressRatio * progressBarWidth)

	bar := strings.Repeat("█", barWidth) + strings.Repeat("-", progressBarWidth-barWidth)
	return fmt.Sprintf("%s: |%s| %.2f%%, Speed: %s, Downloaded: %s / %s", progress.Filename, bar, progressRatio*100, speed, formatSize(downloaded), formatSize(total))
}

func displayProgress(ctx context.Context, progressChans []chan FileProgress, progressStates []FileProgress) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Print("\033[H\033[2J") // 清屏
			for i, progressChan := range progressChans {
				select {
				case progress := <-progressChan:
					progressStates[i] = progress
				default:
				}
			}

			for _, progress := range progressStates {
				fmt.Println(getProgressBar(progress))
			}
		}
	}
}

func monitorProgress(ctx context.Context, progress chan int64, size int64, output string, progressChan chan<- FileProgress) {
	var downloaded int64
	var previousDownloaded int64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentSpeed := downloaded - previousDownloaded
			previousDownloaded = downloaded
			progressChan <- FileProgress{
				Filename:     output,
				Downloaded:   downloaded,
				TotalSize:    size,
				CurrentSpeed: currentSpeed,
			}
		case n, ok := <-progress:
			if !ok {
				progressChan <- FileProgress{
					Filename:   output,
					Downloaded: downloaded,
					TotalSize:  size,
				}
				return
			}
			downloaded += n
		}
	}
}

func parseFilename(url string) string {
	// 从 URL 中解析出文件名
	parts := strings.Split(url, "/")
	name := parts[len(parts)-1]
	if CheckFileName(name) {
		return name
	} else {
		return HandleFileName(name)
	}
}

func CheckFileName(path string) bool {
	var i = 0
	for i = len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' || path[i] == ':' || path[i] == '*' || path[i] == '?' || path[i] == '"' || path[i] == '<' || path[i] == '>' || path[i] == '|' {
			return false
		}

	}
	return true
}

func HandleFileName(path string) string {
	var buffer bytes.Buffer
	var i = 0
	for i = 0; i < len(path); i++ {
		if path[i] == '\\' || path[i] == '/' || path[i] == ':' || path[i] == '*' || path[i] == '?' || path[i] == '"' || path[i] == '<' || path[i] == '>' || path[i] == '|' {
			buffer.WriteByte('_')
		} else {
			buffer.WriteByte(path[i])
		}

	}
	return buffer.String()
}
