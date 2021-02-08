package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v4"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	conn, err := pgx.Connect(context.Background(), os.Getenv("DATABASE"))
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	filesIndexed, err := indexRepositoryFiles(conn, os.Args[1])
	if err != nil {
		return err
	}
	fmt.Println("indexed", filesIndexed, "files")
	return nil
}

func indexRepositoryFiles(conn *pgx.Conn, dir string) (filesIndexed int, err error) {
	/*err = conn.QueryRow(context.Background(), "select name, weight from widgets where id=$1", 42).Scan(&name, &weight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "QueryRow failed: %v\n", err)
		os.Exit(1)
	}*/

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	src := &repositoryFilesSource{repository: dir, files: files}
	_, err = conn.CopyFrom(
		context.Background(),
		pgx.Identifier{"files"},
		[]string{"contents", "filepath"},
		src,
	)
	return len(files), err
}

// repositoryFilesSource implements pgx.CopyFromSource to efficiently bulk-load
// file contents into Postgres.
type repositoryFilesSource struct {
	repository string
	files      []string
}

// Next returns true if there is another row and makes the next row data
// available to Values(). When there are no more rows available or an error
// has occurred it returns false.
func (r *repositoryFilesSource) Next() bool {
	if len(r.files) <= 1 {
		return false
	}
	r.files = r.files[1:]
	return true
}

// Values returns the values for the current row.
func (r *repositoryFilesSource) Values() ([]interface{}, error) {
	contents, err := ioutil.ReadFile(r.files[0])
	if err != nil {
		log.Println(r.files[0], err)
		// ignore error
	}

	// Remove invalid UTF-8 characters, and NULL (illegal UTF8 for Postgres.)
	text := strings.ToValidUTF8(string(contents), "")
	text = strings.Replace(text, "\x00", "", -1)

	return []interface{}{
		[]byte(text),
		[]byte(filepath.Join(r.repository, r.files[0])),
	}, nil
}

// Err returns any error that has been encountered by the CopyFromSource. If
// this is not nil *Conn.CopyFrom will abort the copy.
func (r *repositoryFilesSource) Err() error {
	return nil
}
