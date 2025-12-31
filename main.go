package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

// Sabitler (Constants)
const (
	outputDir     = "scraped"
	reportFile    = "scan_report.log"
	torProxy      = "127.0.0.1:9150" // Tor Browser için (Expert Bundle için 9050 yap)
	timeout       = 90 * time.Second
	maxConcurrent = 5              // Goroutine ile eşzamanlı tarama 
	defaultFile   = "targets.yaml" // Varsayılan dosya (go run main.go için)
)

// İstatistik yapısı (Statistics structure)
type Stats struct {
	Total      int32
	Success    int32
	Failed     int32
	Warnings   int32
	StartTime  time.Time
	TotalBytes int64
}

func main() {
	// Başlık ve bilgilendirme
	printBanner()

	// Komut satırı kontrolü
	targetsFile := defaultFile
	if len(os.Args) >= 2 {
		targetsFile = os.Args[1]
		log.Printf("[INFO] Kullanılan hedef dosyası: %s\n", targetsFile)
	} else {
		log.Printf("[INFO] Dosya adı verilmedi, varsayılan dosya kullanılıyor: %s\n", defaultFile)
	}

	// Çıktı klasörü ve rapor dosyası hazırlığı
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Fatal("[FATAL] Çıktı klasörü oluşturulamadı:", err)
	}
	report, err := os.OpenFile(reportFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal("[FATAL] Rapor dosyası açılamadı:", err)
	}
	defer report.Close()

	// Log çıktısını hem konsola hem dosyaya yaz
	log.SetOutput(io.MultiWriter(os.Stdout, report))
	log.SetFlags(log.Ldate | log.Ltime)

	// Tor client oluştur
	client, err := createTorClient()
	if err != nil {
		log.Fatal("[FATAL] Tor client oluşturulamadı:", err)
	}

	// Tor IP kontrolü yap (OpSec doğrulaması)
	log.Println("[INFO] Tor bağlantısı doğrulanıyor...")
	checkTorConnection(client)

	// Hedefleri dosyadan oku
	targets, err := readTargets(targetsFile)
	if err != nil {
		log.Fatalf("[FATAL] Hedef dosyası okunamadı (%s): %v\n", targetsFile, err)
	}

	// İstatistik yapısını başlat
	stats := &Stats{
		Total:     int32(len(targets)),
		StartTime: time.Now(),
	}

	log.Printf("[INFO] %d adet adres yüklendi. Tarama başlıyor...\n", len(targets))
	log.Printf("[INFO] Maksimum eşzamanlı bağlantı: %d\n", maxConcurrent)
	log.Println(strings.Repeat("-", 80))

	// Goroutine ile paralel tarama (performans için)
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent) // Semaphore ile eşzamanlılık kontrolü

	for _, rawURL := range targets {
		wg.Add(1)
		sem <- struct{}{} // Slot al
		go func(u string) {
			defer wg.Done()
			defer func() { <-sem }() // Slot serbest bırak
			scanTarget(client, u, stats)
		}(rawURL)
	}
	wg.Wait()

	// Sonuçları raporla
	printSummary(stats)
}

func createTorClient() (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", torProxy, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Dial:              dialer.Dial,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

// readTargets - Hedef dosyasını okur (.txt veya .yaml formatı desteklenir)
// Modül 1: Dosya Okuma Modülü (Input Handler)
func readTargets(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var targets []string

	// Dosya uzantısına göre format belirleme
	ext := filepath.Ext(filename)

	if ext == ".yaml" || ext == ".yml" {
		// YAML formatı için
		var yamlTargets []string
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		// YAML içeriğini parse et
		if err := yaml.Unmarshal(data, &yamlTargets); err != nil {
			// Eğer liste formatı değilse, satır satır oku
			log.Println("[WARN] YAML parse hatası, düz metin olarak okunuyor...")
			return readTargetsAsText(filename)
		}

		for _, line := range yamlTargets {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "http") {
				line = "http://" + line
			}
			targets = append(targets, line)
		}
	} else {
		// TXT formatı veya diğer formatlar için satır satır okuma
		return readTargetsAsText(filename)
	}

	return targets, nil
}

// readTargetsAsText - Dosyayı düz metin olarak satır satır okur
func readTargetsAsText(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var targets []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Boş satırları ve yorumları atla
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// HTTP/HTTPS protokolü yoksa ekle
		if !strings.HasPrefix(line, "http") {
			line = "http://" + line
		}

		targets = append(targets, line)
	}

	return targets, scanner.Err()
}

// scanTarget - Tek bir hedefi tarar ve HTML içeriğini kaydeder
// Modül 3: İstek ve Hata Yönetimi + Modül 4: Veri Kayıt
func scanTarget(client *http.Client, rawURL string, stats *Stats) {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("[ERR] Geçersiz URL: %s -> %v\n", rawURL, err)
		atomic.AddInt32(&stats.Failed, 1)
		return
	}

	// HTTP GET isteği oluştur
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		log.Printf("[ERR] İstek oluşturulamadı: %s -> %v\n", rawURL, err)
		atomic.AddInt32(&stats.Failed, 1)
		return
	}

	// Modül 6: User-Agent ve OpSec - Tarayıcı gibi görünmek için header'lar
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("DNT", "1")

	// İsteği gönder ve süreyi ölç
	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)

	if err != nil {
		log.Printf("[ERR] %s -> %v (%.2fs)\n", rawURL, err, duration.Seconds())
		atomic.AddInt32(&stats.Failed, 1)
		return
	}
	defer resp.Body.Close()

	// HTTP durum kodu kontrolü
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN] %s -> HTTP %d (%.2fs)\n", rawURL, resp.StatusCode, duration.Seconds())
		atomic.AddInt32(&stats.Warnings, 1)
		return
	}

	// Response body'yi oku
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ERR] %s -> İçerik okunamadı: %v\n", rawURL, err)
		atomic.AddInt32(&stats.Failed, 1)
		return
	}

	// Dosya adı oluştur (host adından)
	filename := strings.ReplaceAll(u.Host, ":", "_") + ".html"
	filepath := outputDir + "/" + filename

	// HTML içeriğini dosyaya kaydet
	err = os.WriteFile(filepath, body, 0644)
	if err != nil {
		log.Printf("[ERR] %s -> Dosyaya yazılamadı: %v\n", rawURL, err)
		atomic.AddInt32(&stats.Failed, 1)
		return
	}

	// İstatistikleri güncelle (atomic işlemler thread-safe)
	atomic.AddInt32(&stats.Success, 1)
	atomic.AddInt64(&stats.TotalBytes, int64(len(body)))

	log.Printf("[SUCCESS] %s -> %d bytes kaydedildi (%s) (%.2fs)\n",
		rawURL, len(body), filename, duration.Seconds())
}

// printBanner - Program başlangıç banner'ı
func printBanner() {
	banner := `
╔══════════════════════════════════════════════════════════════════════╗
║                       SMK TOR SCRAPER v1.0                           ║
║              Siber Tehdit İstihbaratı Toplama Aracı                  ║
╚══════════════════════════════════════════════════════════════════════╝
[!] Bu araç yalnızca eğitim ve yasal CTI araştırmaları için tasarlanmıştır.
[!] Kullanıcı, aracın kullanımından doğacak yasal sorumluluğu kabul eder.
`
	fmt.Println(banner)
}

// checkTorConnection - Tor bağlantısını doğrular
// Modül 2: Tor Proxy Yönetimi - IP sızıntısı kontrolü
func checkTorConnection(client *http.Client) {
	checkURL := "https://check.torproject.org/api/ip"

	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		log.Printf("[WARN] Tor kontrolü yapılamadı: %v\n", err)
		return
	}

	req.Header.Set("User-Agent", "curl/7.68.0")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[WARN] Tor kontrolü başarısız: %v\n", err)
		log.Println("[WARN] Tor servisinin çalıştığından emin olun (127.0.0.1:9050 veya 9150)")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	responseText := string(body)

	// Basit kontrol: "IsTor":true var mı?
	if strings.Contains(responseText, `"IsTor":true`) {
		log.Println("[OK] ✓ Tor bağlantısı aktif! Trafik Tor ağı üzerinden geçiyor.")

		// IP adresini parse et ve göster
		if strings.Contains(responseText, `"IP"`) {
			parts := strings.Split(responseText, `"IP":"`)
			if len(parts) > 1 {
				ip := strings.Split(parts[1], `"`)[0]
				log.Printf("[OK] ✓ Tor Exit IP: %s\n", ip)
			}
		}
	} else {
		log.Println("[WARN] ✗ Tor bağlantısı algılanamadı! Normal IP kullanılıyor olabilir.")
		log.Println("[WARN] ✗ OpSec riski! Lütfen Tor servisini kontrol edin.")
	}
}

// printSummary - Tarama sonuçlarının özetini yazdırır
// Modül 5: Raporlama ve Loglama
func printSummary(stats *Stats) {
	duration := time.Since(stats.StartTime)

	separator := strings.Repeat("=", 80)
	fmt.Println()
	log.Println(separator)
	log.Println("                          TARAMA ÖZET RAPORU")
	log.Println(separator)
	log.Printf("Toplam Hedef       : %d\n", stats.Total)
	log.Printf("Başarılı           : %d\n", stats.Success)
	log.Printf("Başarısız          : %d\n", stats.Failed)
	log.Printf("Uyarı (HTTP ≠ 200) : %d\n", stats.Warnings)
	log.Printf("Toplam Veri        : %.2f MB\n", float64(stats.TotalBytes)/(1024*1024))
	log.Printf("Toplam Süre        : %s\n", duration.Round(time.Second))
	log.Println(separator)
	log.Printf("Sonuç Klasörü      : ./%s/\n", outputDir)
	log.Printf("Detaylı Log        : ./%s\n", reportFile)
	log.Println(separator)

	// Başarı oranı
	if stats.Total > 0 {
		successRate := float64(stats.Success) / float64(stats.Total) * 100
		log.Printf("Başarı Oranı       : %.1f%%\n", successRate)
	}

	log.Println("\n[INFO] Tarama tamamlandı!")
}
