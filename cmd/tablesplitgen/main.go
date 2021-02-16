package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
)

func main() {
	if len(os.Args) == 1 {
		panic("expected 'create' or 'index' argument")
	}
	if os.Args[1] == "create" {
		incr50k := 0
		for i := 0; i < 200; i++ {
			lowerBound := incr50k
			incr50k += 50000
			upperBound := incr50k
			fmt.Printf("CREATE TABLE files_%03d AS SELECT * FROM files WHERE id > %v AND id < %v;\n", i, lowerBound, upperBound)
		}
	} else if os.Args[1] == "index" {
		var indexCommands []string
		for i := 0; i < 200; i++ {
			indexCommands = append(indexCommands, fmt.Sprintf("CREATE INDEX IF NOT EXISTS files_%03d_contents_trgm_idx ON files_%03d USING GIN (contents gin_trgm_ops);", i, i))
		}
		parallel, err := strconv.ParseInt(os.Getenv("PARALLEL"), 10, 64)
		if err != nil {
			log.Fatal(err)
		}
		runPostgresQueriesInParallel(indexCommands, int(parallel))
	} else if os.Args[1] == "query" {
		query := os.Args[2]
		limit, _ := strconv.ParseInt(os.Args[3], 10, 64)
		tables, _ := strconv.ParseInt(os.Args[4], 10, 64)

		var queriesMu sync.Mutex
		var queries []string
		for i := 0; i < 200; i++ {
			if i+1 > int(tables) {
				break
			}
			var limitStr string
			if limit != 0 {
				limitStr = fmt.Sprintf(" limit %v", limit)
			}
			queries = append(queries, fmt.Sprintf("select count(*) from (select id from files_%03d where contents ~ '%s'"+limitStr+") as e;", i, query))
		}

		workers, err := strconv.ParseInt(os.Getenv("PARALLEL"), 10, 64)
		if err != nil {
			log.Fatal(err)
		}
		start := time.Now()
		done := make(chan struct{}, workers)
		results := make(chan int, workers)
		ctx := context.Background()
		for i := 0; i < int(workers); i++ {
			go func() {
				conn, err := pgx.Connect(ctx, os.Getenv("DATABASE"))
				if err != nil {
					log.Fatal(err)
				}
				defer conn.Close(ctx)

				for {
					queriesMu.Lock()
					if len(queries) == 0 {
						queriesMu.Unlock()
						done <- struct{}{}
						return
					}
					query := queries[0]
					queries = queries[1:]
					queriesMu.Unlock()

					row := conn.QueryRow(ctx, query)
					var numResults int
					if err := row.Scan(&numResults); err != nil {
						log.Println(err)
						continue
					}
					fmt.Println(query, numResults)
					results <- numResults
				}
			}()
		}

		finishedWorkers := 0
		totalResults := 0
		for {
			select {
			case <-done:
				finishedWorkers++
			case gotResults := <-results:
				totalResults += gotResults
			}
			if finishedWorkers == int(workers) {
				break
			}
			if limit > 0 && totalResults >= int(limit) {
				break
			}
		}
		fmt.Printf("%v results in %vms\n", totalResults, time.Since(start).Milliseconds())
	} else {
		panic("expected 'create' or 'index' or 'query' argument")
	}
}

func runPostgresQueriesInParallel(queries []string, workers int) {
	var queriesMu sync.Mutex

	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		go func() {
			for {
				queriesMu.Lock()
				if len(queries) == 0 {
					queriesMu.Unlock()
					done <- struct{}{}
					return
				}
				query := queries[0]
				queries = queries[1:]
				queriesMu.Unlock()

				fmt.Println("query:", query)
				cmd := exec.Command(
					"docker",
					"exec",
					"-it",
					"postgres",
					"psql",
					"-U", "postgres",
					"-P", "pager=off",
					"-c", `\timing`,
					"-c", query,
				)
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
				if err := cmd.Run(); err != nil {
					log.Printf("ERROR running '%s': %v\n", query, err)
				}
			}
		}()
	}

	for i := 0; i < workers; i++ {
		<-done
	}
}
