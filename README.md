# downloader
golang 实现 多线程文件下载，多文件同时下载，文件下载进度同时显示

## 说明
没有实现断点续传，下载失败会重试，重试次数和重试间隔可以通过参数设置
下载过程内容存放在内存中，下载完成后写入文件，所以下载大文件时可能会占用较多内存

## 使用方法
```shell
git clone https://github.com/adiecho/downloader.git
cd downloader
go build
./downloader -u http://vipspeedtest8.wuhan.net.cn:8080/download?size=1073741824,http://vipspeedtest8.wuhan.net.cn:8080/download?size=10737418240 -t 10
```

```shell
Usage of downloader:
  -max_retries int
        max retries for download (default 5)
  -retry_delay duration
        delay between retries (default 2s)
  -t int
        number of threads for each file download (default 4)
  -u string
        urls to download, use ',' to split
  -use_url_filename
        use filename from url or generate downloaded_0, downloaded_1, ... (default true)
```
