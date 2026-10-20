package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

const pagesPerChapter = 50

type databaseConfig struct {
	host     string
	port     string
	user     string
	database string
	password string
	sslMode  string
}

type chapter struct {
	start   int
	end     int
	content string
}

func main() {
	id, outputDir, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	if id == "" {
		id, err = readDocumentID(os.Stdin)
		if err != nil {
			fatal(err)
		}
	}

	config, err := loadDatabaseConfig()
	if err != nil {
		fatal(err)
	}

	db, err := sql.Open("postgres", config.connectionString())
	if err != nil {
		fatal(fmt.Errorf("open database: %w", err))
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fatal(fmt.Errorf("connect to database: %w", err))
	}

	chapters, err := fetchChapters(db, id)
	if err != nil {
		fatal(err)
	}
	if len(chapters) == 0 {
		fatal(fmt.Errorf("no chunks found for document_id %q", id))
	}

	if err := writeChapters(outputDir, chapters); err != nil {
		fatal(err)
	}

	for _, item := range chapters {
		fmt.Printf("wrote %s\n", filepath.Join(outputDir, chapterFilename(item.start, item.end)))
	}
}

func parseArgs(args []string) (documentID, outputDir string, err error) {
	if len(args) > 2 {
		return "", "", errors.New("usage: go run ./cmd/fetch-chapters [document_id] [output_dir]")
	}

	outputDir = "chapters"
	if len(args) >= 1 {
		documentID = strings.TrimSpace(args[0])
	}
	if len(args) == 2 {
		outputDir = strings.TrimSpace(args[1])
		if outputDir == "" {
			return "", "", errors.New("output_dir cannot be empty")
		}
	}
	return documentID, outputDir, nil
}

func readDocumentID(input *os.File) (string, error) {
	fmt.Print("Enter document_id: ")
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read document_id: %w", err)
		}
		return "", errors.New("document_id cannot be empty")
	}

	id := strings.TrimSpace(scanner.Text())
	if id == "" {
		return "", errors.New("document_id cannot be empty")
	}
	return id, nil
}

func loadDatabaseConfig() (databaseConfig, error) {
	config := databaseConfig{
		host:     envOrDefault("LOCAL_HOST", "192.168.1.16"),
		port:     envOrDefault("LOCAL_PORT", "5432"),
		user:     envOrDefault("LOCAL_USER", "kanit"),
		database: envOrDefault("LOCAL_DB", "hono_app"),
		password: envOrDefault("LOCAL_PASS", "4SczbV$udeXcVtT0238Hnnt$"),
		sslMode:  envOrDefault("LOCAL_SSLMODE", "disable"),
	}

	if config.password == "" {
		return databaseConfig{}, errors.New("LOCAL_PASS is required")
	}
	return config, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func (c databaseConfig) connectionString() string {
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.user, c.password),
		Host:     net.JoinHostPort(c.host, c.port),
		Path:     "/" + c.database,
		RawQuery: url.Values{"sslmode": []string{c.sslMode}}.Encode(),
	}
	return dsn.String()
}

func fetchChapters(db *sql.DB, documentID string) ([]chapter, error) {
	rows, err := db.Query(`
		SELECT page_number, content_en
		FROM chunks
		WHERE document_id = $1
		ORDER BY page_number ASC
	`, documentID)
	if err != nil {
		return nil, fmt.Errorf("fetch chunks: %w", err)
	}
	defer rows.Close()

	byRange := make(map[int]*chapter)
	for rows.Next() {
		var pageNumber int64
		var content sql.NullString
		if err := rows.Scan(&pageNumber, &content); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		if pageNumber < 1 || pageNumber > int64(^uint(0)>>1) {
			return nil, fmt.Errorf("invalid page_number %d", pageNumber)
		}

		page := int(pageNumber)
		start := ((page-1)/pagesPerChapter)*pagesPerChapter + 1
		item := byRange[start]
		if item == nil {
			item = &chapter{start: start, end: start + pagesPerChapter - 1}
			byRange[start] = item
		}
		if content.Valid && content.String != "" {
			if item.content != "" {
				item.content += "\n"
			}
			item.content += content.String
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunks: %w", err)
	}

	starts := make([]int, 0, len(byRange))
	for start := range byRange {
		starts = append(starts, start)
	}
	sort.Ints(starts)

	chapters := make([]chapter, 0, len(starts))
	for _, start := range starts {
		chapters = append(chapters, *byRange[start])
	}
	return chapters, nil
}

func writeChapters(outputDir string, chapters []chapter) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	for _, item := range chapters {
		filename := filepath.Join(outputDir, chapterFilename(item.start, item.end))
		if err := os.WriteFile(filename, []byte(item.content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}
	return nil
}

func chapterFilename(start, end int) string {
	return strconv.Itoa(start) + "-" + strconv.Itoa(end) + ".txt"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
