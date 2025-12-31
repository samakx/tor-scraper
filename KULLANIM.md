# Kullanım Kılavuzu

## Hızlı Başlangıç

### 1. Tor'u Başlat

**Windows:**
Tor Browser'ı indir (torproject.org) ve çalıştır. Bağlantı kurulunca hazır (Port 9150).

**Linux:**
```bash
sudo apt install tor
sudo systemctl start tor
# Port 9050'de çalışır
```

Port farklıysa main.go'daki `torProxy` değişkenini değiştir.

### 2. Hedef Listesi Hazırla

targets.yaml dosyası oluştur:
```yaml
# Yorumlar # ile başlar
https://check.torproject.org
http://örnekonion.onion
```

### 3. Çalıştır

```bash
go mod download
go run main.go
```

Başka dosya kullanmak için:
```bash
go run main.go hedeflerim.txt
```

## Çıktılar

- **scraped/** klasörü: İndirilen HTML dosyaları
- **scan_report.log**: Detaylı tarama logu

## Özelleştirme

main.go dosyasındaki sabitleri değiştir:

```go
const torProxy = "127.0.0.1:9050"  // Linux için
const timeout = 120 * time.Second  // Daha uzun timeout
const maxConcurrent = 10           // Daha hızlı tarama
```

## Sorunlar ve Çözümler

**"Tor client oluşturulamadı"**
Tor Browser açık mı kontrol et. Linux'ta `sudo systemctl status tor` komutuyla bak.

**"YAML parse hatası"**
Dosyayı .txt olarak kaydet veya yorumları ayrı satırlara yaz.

**Çok fazla timeout**
Tor ağı yavaş olabilir. timeout süresini artır veya maxConcurrent değerini düşür (3-5 arası).

**IP kontrolü başarısız**
Tor bağlantısını test et: Tarayıcıdan check.torproject.org aç, "Congratulations" mesajını görmelisin.

## Binary Derleme

```bash
# Windows
go build -o tor-scraper.exe main.go

# Linux
go build -o tor-scraper main.go
```

## Notlar

- .onion sitelerin yüklenmesi uzun sürebilir (30-90 saniye normal)
- Goroutines varsayılan 5 eşzamanlı bağlantı yapıyor, performans için artırılabilir
- Program çalışırken Tor Browser'ı kapatma
- Çok fazla eşzamanlı istek Tor ağını yavaşlatabilir
