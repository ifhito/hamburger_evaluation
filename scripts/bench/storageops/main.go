// 写真ストレージ候補 1 社分を検証する。
//
//	usage: S3_ENDPOINT=... S3_BUCKET=... S3_ACCESS_KEY_ID=... S3_SECRET_ACCESS_KEY=... \
//	       PUBLIC_BASE_URL=... ITERATIONS=50 go run .
//
// backend-go の adapter/storage/s3.go と**同じ組み立て方**で client を作る。
// Region は "auto" 固定、path-style アドレッシング。ここが各社で通るかが検証の核心。
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAILED:", err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	ak := os.Getenv("S3_ACCESS_KEY_ID")
	sk := os.Getenv("S3_SECRET_ACCESS_KEY")
	base := os.Getenv("PUBLIC_BASE_URL")
	if endpoint == "" || bucket == "" || ak == "" || sk == "" {
		return fmt.Errorf("S3_ENDPOINT / S3_BUCKET / S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY が要る")
	}
	n := 50
	if v := os.Getenv("ITERATIONS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			n = p
		}
	}

	ctx := context.Background()

	// backend-go/internal/adapter/storage/s3.go の NewS3 と同じ組み立て。
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(ak, sk, ""),
		UsePathStyle: true,
	})

	// 写真の代わりに、縮小後と同じくらいの大きさ(長辺 1600px の JPEG 相当)を使う。
	payload := make([]byte, 300*1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	key := fmt.Sprintf("bench/%d.bin", time.Now().UnixNano())

	fmt.Println("## 接続と Region の扱い")
	fmt.Println()

	// --- PUT を 1 回試して、Region "auto" が通るか確かめる ---
	t0 := time.Now()
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(payload),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		msg := err.Error()
		fmt.Printf("- `Region: \"auto\"` の PUT: **失敗**\n")
		fmt.Printf("- エラー: `%v`\n", err)
		fmt.Println()
		switch {
		case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"),
			strings.Contains(msg, "timeout"), strings.Contains(msg, "TLS"):
			fmt.Println("**到達できていない。** エンドポイントの URL とネットワークを確認すること。")
		case strings.Contains(msg, "SignatureDoesNotMatch"), strings.Contains(msg, "AuthorizationHeaderMalformed"),
			strings.Contains(msg, "region"), strings.Contains(msg, "Region"):
			fmt.Println("**署名かリージョンの問題。** `adapter/storage/s3.go` は `Region: \"auto\"` を")
			fmt.Println("ハードコードしている。この事業者が実リージョンを要求するなら、設定に出す改修が要る。")
		case strings.Contains(msg, "NoSuchBucket"), strings.Contains(msg, "AccessDenied"):
			fmt.Println("**バケットか権限の問題。** バケット名と、キーに書き込み権限があるかを確認すること。")
		default:
			fmt.Println("原因は上のエラーを参照。")
		}
		return fmt.Errorf("PutObject: %w", err)
	}
	fmt.Printf("- `Region: \"auto\"` の PUT: **成功**(%.0fms)\n", float64(time.Since(t0).Milliseconds()))
	fmt.Printf("- path-style アドレッシング: 成功\n")
	fmt.Println()

	// --- PUT のレイテンシ ---
	puts := make([]time.Duration, 0, n)
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("bench/%d-%d.bin", time.Now().UnixNano(), i)
		s := time.Now()
		_, err := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(k),
			Body: bytes.NewReader(payload), ContentType: aws.String("application/octet-stream"),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "put %d: %v\n", i, err)
			break
		}
		puts = append(puts, time.Since(s))
		keys = append(keys, k)
	}

	// --- 公開 URL からの GET ---
	var gets []time.Duration
	publicOK := false
	if base != "" && len(keys) > 0 {
		url := base + "/" + keys[0]
		resp, err := http.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			publicOK = resp.StatusCode == 200
		}
		if publicOK {
			gets = make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				s := time.Now()
				r, err := http.Get(url)
				if err != nil {
					break
				}
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				gets = append(gets, time.Since(s))
			}
		}
	}

	// --- DELETE ---
	dels := make([]time.Duration, 0, len(keys))
	for _, k := range keys {
		s := time.Now()
		if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(k),
		}); err != nil {
			break
		}
		dels = append(dels, time.Since(s))
	}
	_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})

	fmt.Println("## レイテンシ(300KiB の物体、日本から)")
	fmt.Println()
	fmt.Println("| 操作 | n | 平均 | 標準偏差 | p50 | p90 | p95 | p99 | 最小 | 最大 |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	row("PUT (書き込み)", puts)
	if publicOK {
		row("GET (公開 URL から配信)", gets)
	}
	row("DELETE (削除)", dels)
	fmt.Println()
	if base == "" {
		fmt.Println("- 公開 URL(`PUBLIC_BASE_URL`)が未設定のため、配信は未計測。")
	} else if !publicOK {
		fmt.Printf("- 公開 URL からの取得に**失敗**した。バケットの公開設定を確認すること(`%s`)。\n", base)
	} else {
		fmt.Printf("- 公開配信: 成功(`%s`)\n", base)
	}
	fmt.Printf("- 転送量の目安: PUT %d 回 + GET %d 回 = 約 %.1f MiB\n",
		len(puts), len(gets), float64((len(puts)+len(gets))*len(payload))/1024/1024)
	return nil
}

func row(name string, ds []time.Duration) {
	if len(ds) == 0 {
		fmt.Printf("| %s | 0 | 失敗 | | | | | | | |\n", name)
		return
	}
	d := append([]time.Duration{}, ds...)
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	fmt.Printf("| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
		name, len(d), ms(mean(d)), ms(stddev(d)),
		ms(pct(d, .50)), ms(pct(d, .90)), ms(pct(d, .95)), ms(pct(d, .99)), ms(d[0]), ms(d[len(d)-1]))
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }

func mean(d []time.Duration) time.Duration {
	var t time.Duration
	for _, x := range d {
		t += x
	}
	return t / time.Duration(len(d))
}

func stddev(d []time.Duration) time.Duration {
	m := float64(mean(d))
	var s float64
	for _, x := range d {
		s += (float64(x) - m) * (float64(x) - m)
	}
	return time.Duration(math.Sqrt(s / float64(len(d))))
}

func pct(sorted []time.Duration, p float64) time.Duration {
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}
