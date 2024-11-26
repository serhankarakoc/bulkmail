package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

var (
	mu          sync.Mutex
	sentCount   int
	totalEmails int
)

// E-posta gönderme fonksiyonu
func sendEmail(to []string, subject, body, smtpServer, smtpPort, username, password string) error {
	msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nMIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n%s", username, to[0], subject, body)
	auth := smtp.PlainAuth("", username, password, smtpServer)
	return smtp.SendMail(smtpServer+":"+smtpPort, auth, username, to, []byte(msg))
}

// Toplu e-posta gönderimi
func sendBulkEmails(recipients []string, subject, body string, delay time.Duration, smtpServer, smtpPort, username, password string) {
	for _, recipient := range recipients {
		if err := sendEmail([]string{recipient}, subject, body, smtpServer, smtpPort, username, password); err != nil {
			log.Printf("Failed to send email to %s: %s", recipient, err)
		} else {
			log.Printf("Email sent to %s successfully.", recipient)
		}
		mu.Lock()
		sentCount++
		mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// Excel dosyasından e-posta adreslerini okuma
func readEmailsFromExcel(filePath, sheetName string) ([]string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	emails := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) > 0 {
			emails = append(emails, row[0])
		}
	}
	return emails, nil
}

// Şablon dosyasını okuma
func readTemplateFromFile(templatePath string) (string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// E-posta gönderim yükleme işlemi
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.ServeFile(w, r, "upload.html")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Dosya yükleme hatası", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tempFile, err := os.CreateTemp("", "uploaded-*.xlsx")
	if err != nil {
		http.Error(w, "Dosya kaydedilemedi", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	if _, err := io.Copy(tempFile, file); err != nil {
		http.Error(w, "Dosya kopyalama hatası", http.StatusInternalServerError)
		return
	}

	sheetName := r.FormValue("sheetName")
	smtpServer := r.FormValue("smtpServer")
	smtpPort := r.FormValue("smtpPort")
	username := r.FormValue("username")
	password := r.FormValue("password")

	templateFile, _, err := r.FormFile("templateFile")
	if err != nil {
		http.Error(w, "Şablon dosyası yüklenemedi", http.StatusBadRequest)
		return
	}
	defer templateFile.Close()
	tempTemplateFile, err := os.CreateTemp("", "template-*.html")
	if err != nil {
		http.Error(w, "Şablon dosyası kaydedilemedi", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempTemplateFile.Name())
	defer tempTemplateFile.Close()
	if _, err := io.Copy(tempTemplateFile, templateFile); err != nil {
		http.Error(w, "Şablon dosyası kopyalama hatası", http.StatusInternalServerError)
		return
	}
	templateContent, err := readTemplateFromFile(tempTemplateFile.Name())
	if err != nil {
		http.Error(w, "Şablon dosyası okunamadı", http.StatusInternalServerError)
		return
	}

	emails, err := readEmailsFromExcel(tempFile.Name(), sheetName)
	if err != nil {
		http.Error(w, "Excel okuma hatası", http.StatusInternalServerError)
		return
	}
	totalEmails = len(emails)
	sentCount = 0

	go sendBulkEmails(emails, "Toplu E-posta Konusu", templateContent, 1*time.Minute, smtpServer, smtpPort, username, password)

	http.Redirect(w, r, "/progress", http.StatusSeeOther)
}

// Progress durumu gösterimi
func progressHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	progress := float64(sentCount) / float64(totalEmails) * 100
	mu.Unlock()
	tmpl := template.Must(template.ParseFiles("progress.html"))
	tmpl.Execute(w, struct {
		Total     int
		SentCount int
		Progress  float64
	}{
		Total:     totalEmails,
		SentCount: sentCount,
		Progress:  progress,
	})
}

func main() {
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/progress", progressHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "upload.html")
	})

	fmt.Println("Sunucu 8080 portunda çalışıyor...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
