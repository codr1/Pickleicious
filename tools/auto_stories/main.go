package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// Story represents a user story to be created as a GitHub issue
type Story struct {
	ID             string   `yaml:"id"`
	Category       string   `yaml:"category"`
	Title          string   `yaml:"title"`
	Description    string   `yaml:"description"`
	Acceptance     []any    `yaml:"acceptance"`
	Labels         []string `yaml:"labels"`
	Assignee       string   `yaml:"assignee"`
	Dependencies   []string `yaml:"dependencies"`
	Implementation string   `yaml:"implementation,omitempty"`
}

// Stories is a collection of Story objects
type Stories struct {
	Stories []Story `yaml:"stories"`
}

func main() {
	var (
		token = flag.String(
			"token",
			os.Getenv("GITHUB_API_KEY"),
			"GitHub API token (or set GITHUB_API_KEY env var)",
		)
		owner      = flag.String("owner", "", "Repository owner")
		repo       = flag.String("repo", "", "Repository name")
		filepath   = flag.String("file", "", "Path to YAML file containing stories")
		projectNum = flag.Int("project", 0, "Project number")
	)

	flag.Parse()

	if *token == "" || *owner == "" || *repo == "" || *filepath == "" {
		log.Fatal("All flags must be provided")
	}

	// Read and parse stories
	stories, err := loadStories(*filepath)
	if err != nil {
		log.Fatalf("Failed to load stories: %v", err)
	}

	// Create GraphQL client
	ctx := context.Background()
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := graphql.NewClient("https://api.github.com/graphql", httpClient)

	// First, get the project ID
	var projectQuery struct {
		User struct {
			ProjectV2 struct {
				ID string
			} `graphql:"projectV2(number: $projectNum)"`
		} `graphql:"user(login: $login)"`
	}
	variables := map[string]interface{}{
		"login":      graphql.String(*owner),
		"projectNum": graphql.Int(*projectNum),
	}
	if err := client.Query(ctx, &projectQuery, variables); err != nil {
		log.Fatalf("Failed to get project ID: %v", err)
	}
	projectID := projectQuery.User.ProjectV2.ID

	// Create project items for each story
	for _, story := range stories.Stories {
		itemID, err := createProjectItem(ctx, client, projectID, story)
		if err != nil {
			log.Printf("Failed to create project item for story %s: %v", story.ID, err)
			continue
		}
		log.Printf("Created project item %s for story %s", itemID, story.ID)

		// Rate limit consideration
		time.Sleep(time.Second)
	}
}

// Simple token transport for GitHub API authentication
type tokenTransport struct {
	token string
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "token "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}

func loadStories(path string) (*Stories, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var stories Stories
	if err := yaml.Unmarshal(data, &stories); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &stories, nil
}

func createProjectItem(ctx context.Context, client *graphql.Client, projectID string, story Story) (string, error) {
	// Build issue body
	body := strings.Builder{}
	body.WriteString(fmt.Sprintf("**Story ID:** %s\n\n", story.ID))
	body.WriteString(fmt.Sprintf("**Category:** %s\n\n", story.Category))
	body.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", story.Description))

	body.WriteString("**Acceptance Criteria:**\n")
	for _, criterion := range story.Acceptance {
		switch v := criterion.(type) {
		case string:
			body.WriteString(fmt.Sprintf("- [ ] %s\n", v))
		case map[string]interface{}:
			for key, value := range v {
				body.WriteString(fmt.Sprintf("- [ ] %s\n", key))
				switch items := value.(type) {
				case []interface{}:
					for _, item := range items {
						switch i := item.(type) {
						case string:
							body.WriteString(fmt.Sprintf("    - %s\n", i))
						case map[string]interface{}:
							for k, v := range i {
								body.WriteString(fmt.Sprintf("    - %s:\n", k))
								if subList, ok := v.([]interface{}); ok {
									for _, subItem := range subList {
										body.WriteString(fmt.Sprintf("        - %s\n", subItem))
									}
								}
							}
						}
					}
				}
			}
		}
	}
	body.WriteString("\n")

	if len(story.Dependencies) > 0 {
		body.WriteString("**Dependencies:**\n")
		for _, dep := range story.Dependencies {
			body.WriteString(fmt.Sprintf("- %s\n", dep))
		}
		body.WriteString("\n")
	}

	var mutation struct {
		CreateProjectV2Item struct {
			Item struct {
				ID string
			}
		} `graphql:"createProjectV2Item(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"projectId": projectID,
			"title":     fmt.Sprintf("[%s] %s", story.ID, story.Title),
			"body":      body.String(),
		},
	}

	if err := client.Mutate(ctx, &mutation, variables); err != nil {
		return "", fmt.Errorf("failed to create project item: %w", err)
	}

	return mutation.CreateProjectV2Item.Item.ID, nil
}
