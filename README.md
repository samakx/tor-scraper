# TOR Scraper

Siber tehdit istihbaratı toplama amacıyla geliştirilmiş bir Go uygulaması. Tor ağı üzerinden .onion adreslerini toplu olarak tarayıp HTML içeriklerini kaydeder.

## Gereksinimler

- Go 1.23 veya üzeri
- Tor Browser (Windows için 9150 portu) veya Tor servisi (Linux için 9050 portu)

## Kurulum ve Çalıştırma

```bash
# Bağımlılıkları indir
go mod download

# Programı çalıştır
go run main.go

# Binary derle (isteğe bağlı)
go build -o tor-scraper.exe main.go
```

## Kullanım

Program başladığında `targets.yaml` dosyasındaki adresleri okur ve sırayla tarar. Sonuçlar `scraped/` klasöründe HTML dosyaları olarak, detaylı log ise `scan_report.log` dosyasında saklanır.

Farklı bir hedef dosyası kullanmak için:
```bash
go run main.go hedefler.txt
```

## Hedef Dosyası Formatı

targets.yaml:
```yaml
# Yorum satırları # ile başlar
https://check.torproject.org
http://siteonionadresi.onion
```

## Proje Yapısı

```
tor-scraper/
├── main.go              # Ana program
├── go.mod               # Go modül tanımı
├── go.sum               # Go bağımlılık hash'leri
├── targets.yaml         # Hedef listesi (YAML)
├── targets.txt          # Hedef listesi (TXT)
├── scan_report.log      # Detaylı tarama logu
├── scraped/             # İndirilen HTML dosyaları
│   ├── check.torproject.org.html
│   ├── duckduckgo...onion.html
│   └── ...
└── README.md            # Bu dosya
```

**4 Ana Modül:**

1. **Dosya Okuma**: targets.yaml veya .txt dosyasından URL'leri okur, boş satır ve yorumları temizler
2. **Tor Proxy Yönetimi**: SOCKS5 proxy üzerinden http.Client oluşturur, IP sızıntısı kontrolü yapar
3. **İstek ve Hata Yönetimi**: Her hedefi tarar, hatalar programı durdurmaz, timeout 90 saniye
4. **Veri Kayıt**: HTML içerikleri scraped/ klasörüne kaydedilir, detaylı istatistikler üretilir

**Ek Özellikler:**

- Goroutines ile paralel tarama (varsayılan 5 eşzamanlı bağlantı)
- User-Agent spoofing ile browser gibi görünme
- Atomic operations ile thread-safe istatistik tutma
- Başarı/başarısızlık durumlarının detaylı raporlanması

## Ayarlar

main.go dosyasındaki sabitleri değiştirerek ayarlayabilirsiniz:

```go
const torProxy = "127.0.0.1:9150"  // Tor Browser için 9150, Tor servisi için 9050
const timeout = 90 * time.Second   // İstek zaman aşımı süresi
const maxConcurrent = 5            // Eşzamanlı bağlantı sayısı
```

## Çıktı Örneği

```
Toplam Hedef       : 8
Başarılı           : 5
Başarısız          : 3
Toplam Veri        : 0.81 MB
Toplam Süre        : 1m30s
Başarı Oranı       : 62.5%
```

## Sorun Giderme

**Tor bağlantısı başarısız**: Tor Browser'ın açık ve bağlı olduğundan emin olun.

**YAML parse hatası**: Dosyayı .txt formatında kaydedin veya yorumları ayrı satırlara yazın.

**Timeout hataları**: Tor ağı yavaş olabilir, timeout süresini artırın veya maxConcurrent değerini düşürün.

## Yasal Uyarı

Bu araç yalnızca eğitim ve yasal CTI araştırmaları için geliştirilmiştir. İzinsiz sistem taraması, kişisel veri toplama veya yasadışı aktiviteler için kullanılamaz. Kullanıcı tüm yasal sorumluluğu kabul eder.
