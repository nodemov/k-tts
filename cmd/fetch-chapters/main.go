package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

const usage = "usage: go run ./cmd/fetch-chapters <book_id> [from to] [output_dir]"

type chapter struct {
	index int
	title string
	text  string
}

type fetchOptions struct {
	bookID    string
	outputDir string
	from      int // 1-based inclusive; 0 means no lower bound
	to        int // 1-based inclusive; 0 means no upper bound
}

func main() {
	if err := loadDotEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	if opts.bookID == "" {
		opts.bookID, err = readBookID(os.Stdin)
		if err != nil {
			fatal(err)
		}
	}

	dsn, err := loadDatabaseURL()
	if err != nil {
		fatal(err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fatal(fmt.Errorf("open database: %w", err))
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fatal(fmt.Errorf("connect to database: %w", err))
	}

	bookTitle, err := fetchBookTitle(db, opts.bookID)
	if err != nil {
		fatal(err)
	}

	chapters, err := fetchThaiChapters(db, opts)
	if err != nil {
		fatal(err)
	}
	if len(chapters) == 0 {
		if opts.from > 0 || opts.to > 0 {
			fatal(fmt.Errorf("no Thai-translated chapters found for book id %q in range %s", opts.bookID, formatRange(opts.from, opts.to)))
		}
		fatal(fmt.Errorf("no Thai-translated chapters found for book id %q", opts.bookID))
	}

	if err := writeChapters(opts.outputDir, chapters); err != nil {
		fatal(err)
	}

	fmt.Printf("book: %s (%s)\n", bookTitle, opts.bookID)
	if opts.from > 0 || opts.to > 0 {
		fmt.Printf("range: %s\n", formatRange(opts.from, opts.to))
	}
	fmt.Printf("wrote %d Thai chapters to %s\n", len(chapters), opts.outputDir)
	for _, item := range chapters {
		fmt.Printf("  %s\n", filepath.Join(opts.outputDir, chapterFilename(item.index)))
	}
}

func parseArgs(args []string) (fetchOptions, error) {
	opts := fetchOptions{outputDir: "chapters"}
	if len(args) > 4 {
		return fetchOptions{}, errors.New(usage)
	}
	if len(args) >= 1 {
		opts.bookID = strings.TrimSpace(args[0])
	}
	if len(args) == 0 || len(args) == 1 {
		return opts, nil
	}

	if len(args) == 2 {
		second := strings.TrimSpace(args[1])
		if second == "" {
			return fetchOptions{}, errors.New("output_dir cannot be empty")
		}
		if _, err := strconv.Atoi(second); err == nil {
			return fetchOptions{}, fmt.Errorf("%s\nprovide both from and to, e.g. <book_id> 1 100", usage)
		}
		opts.outputDir = second
		return opts, nil
	}

	from, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil {
		return fetchOptions{}, fmt.Errorf("%s\nfrom must be an integer", usage)
	}
	to, err := strconv.Atoi(strings.TrimSpace(args[2]))
	if err != nil {
		return fetchOptions{}, fmt.Errorf("%s\nto must be an integer", usage)
	}
	if from < 1 || to < 1 {
		return fetchOptions{}, errors.New("from and to must be >= 1")
	}
	if from > to {
		return fetchOptions{}, fmt.Errorf("from (%d) cannot be greater than to (%d)", from, to)
	}
	opts.from = from
	opts.to = to

	if len(args) == 4 {
		opts.outputDir = strings.TrimSpace(args[3])
		if opts.outputDir == "" {
			return fetchOptions{}, errors.New("output_dir cannot be empty")
		}
	}
	return opts, nil
}

func formatRange(from, to int) string {
	switch {
	case from > 0 && to > 0:
		return fmt.Sprintf("%d-%d", from, to)
	case from > 0:
		return fmt.Sprintf("%d-", from)
	case to > 0:
		return fmt.Sprintf("-%d", to)
	default:
		return "all"
	}
}

func readBookID(input *os.File) (string, error) {
	fmt.Print("Enter book id: ")
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read book id: %w", err)
		}
		return "", errors.New("book id cannot be empty")
	}

	id := strings.TrimSpace(scanner.Text())
	if id == "" {
		return "", errors.New("book id cannot be empty")
	}
	return id, nil
}

func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for lineNum, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: invalid line", path, lineNum+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNum+1)
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		// Do not override variables already set in the environment.
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return nil
}

func loadDatabaseURL() (string, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return "", errors.New("DATABASE_URL is required (set in .env or environment)")
	}
	return ensureSSLMode(dsn)
}

func ensureSSLMode(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	query := parsed.Query()
	if query.Get("sslmode") == "" {
		query.Set("sslmode", "disable")
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func fetchBookTitle(db *sql.DB, bookID string) (string, error) {
	var title sql.NullString
	err := db.QueryRow(`
		SELECT COALESCE(NULLIF(custom_title, ''), NULLIF(title, ''), id)
		FROM books
		WHERE id = $1
	`, bookID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("book id %q not found", bookID)
	}
	if err != nil {
		return "", fmt.Errorf("fetch book: %w", err)
	}
	if !title.Valid || title.String == "" {
		return bookID, nil
	}
	return title.String, nil
}

func fetchThaiChapters(db *sql.DB, opts fetchOptions) ([]chapter, error) {
	// chapter_index is 0-based; CLI from/to are 1-based (same as output filenames).
	query := `
		SELECT
			c.chapter_index,
			COALESCE(NULLIF(c.metadata_json::jsonb->>'translatedTitle', ''), c.title) AS title,
			COALESCE(c.metadata_json::jsonb->>'translatedText', '') AS text
		FROM chapters c
		JOIN books b ON b.url = c.book_url
		WHERE b.id = $1
		  AND c.status_translate = 'done'
		  AND COALESCE(c.metadata_json::jsonb->>'translatedText', '') <> ''
	`
	args := []any{opts.bookID}
	if opts.from > 0 {
		args = append(args, opts.from-1)
		query += fmt.Sprintf(" AND c.chapter_index >= $%d", len(args))
	}
	if opts.to > 0 {
		args = append(args, opts.to-1)
		query += fmt.Sprintf(" AND c.chapter_index <= $%d", len(args))
	}
	query += " ORDER BY c.chapter_index ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch chapters: %w", err)
	}
	defer rows.Close()

	var chapters []chapter
	for rows.Next() {
		var item chapter
		if err := rows.Scan(&item.index, &item.title, &item.text); err != nil {
			return nil, fmt.Errorf("scan chapter: %w", err)
		}
		chapters = append(chapters, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chapters: %w", err)
	}
	return chapters, nil
}

func writeChapters(outputDir string, chapters []chapter) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	for _, item := range chapters {
		filename := filepath.Join(outputDir, chapterFilename(item.index))
		content := formatChapterContent(item)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}
	return nil
}

func formatChapterContent(item chapter) string {
	title := strings.TrimSpace(item.title)
	text := strings.TrimSpace(item.text)
	if title == "" {
		return text + "\n"
	}
	if text == "" {
		return title + "\n"
	}
	return title + "\n\n" + text + "\n"
}

func chapterFilename(index int) string {
	// chapter_index is 0-based; files are 1-based and zero-padded for lexical sort
	return fmt.Sprintf("%04d.txt", index+1)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
