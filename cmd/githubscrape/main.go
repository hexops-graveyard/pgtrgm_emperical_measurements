package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

var (
	minStarsFlag = flag.Int("min-stars", 1000, "minimum stars for inclusion")
	languageFlag = flag.String("language", "Go", "language to target")
)

func main() {
	flag.Parse()

	err := run()
	if err != nil {
		log.Println(err)
	}
}

func run() error {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("ACCESS_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)
	repos, err := topReposForLanguage(client, *languageFlag, *minStarsFlag)
	if err != nil {
		return err
	}
	printJSON(repos)
	return nil
}

func topReposForLanguage(client *githubv4.Client, language string, minStarsThreshold int) ([]string, error) {
	query := fmt.Sprintf(`language:%s stars:"> %v"`, language, minStarsThreshold)
	responses := searchReposAll(client, query)
	var list []string
	for {
		resp, ok := <-responses
		if !ok {
			break
		}
		if resp.err != nil {
			return list, resp.err
		}
		for _, edge := range resp.resp.Edges {
			list = append(list, string(edge.Node.Repository.Url))
		}
		log.Println("fetched", len(list), "of", resp.resp.RepositoryCount)
	}
	return list, nil
}

type searchReposAllResponse struct {
	resp *searchReposResponse
	err  error
}

func searchReposAll(client *githubv4.Client, query string) chan searchReposAllResponse {
	ch := make(chan searchReposAllResponse)
	go func() {
		var resp *searchReposResponse
		for resp == nil || resp.PageInfo.HasNextPage {
			var after string
			if resp != nil {
				after = string(resp.PageInfo.EndCursor)
			}
			first := 100
			var err error
			resp, err = searchRepos(client, query, first, after)
			if err != nil {
				ch <- searchReposAllResponse{err: err}
				close(ch)
				return
			}
			ch <- searchReposAllResponse{resp: resp}
		}
		close(ch)
	}()
	return ch
}

type searchReposResponse struct {
	RepositoryCount githubv4.Int
	PageInfo        struct {
		EndCursor   githubv4.String
		HasNextPage githubv4.Boolean
	}
	Edges []struct {
		Node struct {
			Repository struct {
				Url githubv4.String
			} `graphql:"... on Repository"`
		}
	}
}

func searchRepos(client *githubv4.Client, query string, first int, after string) (*searchReposResponse, error) {
	var q struct {
		Search searchReposResponse `graphql:"search(query: $query, first: $first, after: $after, type: REPOSITORY)"`
	}
	variables := map[string]interface{}{
		"query": githubv4.String(query),
		"first": githubv4.Int(first),
		"after": githubv4.String(after),
	}
	if after == "" {
		variables["after"] = (*githubv4.String)(nil)
	}
	err := client.Query(context.Background(), &q, variables)
	if err != nil {
		return nil, err
	}
	return &q.Search, nil
}

// printJSON prints v as JSON encoded with indent to stdout. It panics on any error.
func printJSON(v interface{}) {
	w := json.NewEncoder(os.Stdout)
	w.SetIndent("", "\t")
	err := w.Encode(v)
	if err != nil {
		panic(err)
	}
}
