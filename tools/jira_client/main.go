package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// Flags for subcommands
var (
	projectKey     string
	issueKey       string
	summary        string
	description    string
	issueType      string
	commentBody    string
	transitionName string
	resolutionName string
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "jira-cli",
	Short: "A CLI tool to interact with Jira",
	Long: `jira-cli is a command line interface tool that uses the go-jira library 
to list, create, and update Jira tickets. Jira domain, email, and API key 
must be provided as environment variables:
  
  JIRA_DOMAIN, JIRA_EMAIL, JIRA_API_KEY
`,
}

// listCmd lists the tickets in a project
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets in a project",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := getJiraClient()
		if err != nil {
			log.Fatalf("Error creating Jira client: %v", err)
		}

		if projectKey == "" {
			log.Fatal("Project key is required (use --projectKey)")
		}

		jql := fmt.Sprintf("project = %s ORDER BY created DESC", projectKey)
		issues, _, err := client.Issue.SearchWithContext(context.Background(), jql, nil)
		if err != nil {
			log.Fatalf("Error fetching tickets: %v", err)
		}

		fmt.Printf("Tickets in project %s:\n", projectKey)
		if len(issues) == 0 {
			fmt.Println("No tickets found.")
			return
		}

		for _, issue := range issues {
			fmt.Printf("- %s: %s\n", issue.Key, issue.Fields.Summary)
		}
	},
}

// createCmd creates a new ticket in a project
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket in a project",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := getJiraClient()
		if err != nil {
			log.Fatalf("Error creating Jira client: %v", err)
		}

		if projectKey == "" {
			log.Fatal("Project key is required (use --projectKey)")
		}
		if summary == "" {
			log.Fatal("Summary is required (use --summary)")
		}
		if issueType == "" {
			issueType = "Task" // default to "Task" if not provided
		}

		issue := &jira.Issue{
			Fields: &jira.IssueFields{
				Project:     jira.Project{Key: projectKey},
				Summary:     summary,
				Description: description,
				Type: jira.IssueType{
					Name: issueType,
				},
			},
		}

		newIssue, resp, err := client.Issue.Create(issue)
		if err != nil {
			log.Fatalf("Error creating ticket: %v\nResponse: %v", err, resp)
		}

		fmt.Printf("Ticket created! %v\n", newIssue)
	},
}

// updateCmd updates an existing ticket
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing ticket",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := getJiraClient()
		if err != nil {
			log.Fatalf("Error creating Jira client: %v", err)
		}

		if issueKey == "" {
			log.Fatal("Issue key is required (use --issueKey)")
		}

		// Fetch the existing issue
		issue, resp, err := client.Issue.Get(issueKey, nil)
		if err != nil {
			log.Fatalf("Error fetching issue %s: %v\nResponse: %v", issueKey, err, resp)
		}

		// Update only if flags were provided
		if summary != "" {
			issue.Fields.Summary = summary
		}
		if description != "" {
			issue.Fields.Description = description
		}

		updatedIssue, resp, err := client.Issue.Update(issue)
		if err != nil {
			log.Fatalf("Error updating issue %s: %v\nResponse: %v", issueKey, err, resp)
		}

		fmt.Printf("Ticket updated! Key: %s  Summary: %s\n", updatedIssue.Key, updatedIssue.Fields.Summary)
	},
}

// commentCmd adds a comment to an existing ticket
var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Add a comment to an existing ticket",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := getJiraClient()
		if err != nil {
			log.Fatalf("Error creating Jira client: %v", err)
		}

		if issueKey == "" {
			log.Fatal("Issue key is required (use --issueKey)")
		}
		if commentBody == "" {
			log.Fatal("Comment body is required (use --body)")
		}

		comment := &jira.Comment{
			Body: commentBody,
		}

		newComment, resp, err := client.Issue.AddComment(issueKey, comment)
		if err != nil {
			log.Fatalf("Error adding comment to issue %s: %v\nResponse: %v", issueKey, err, resp)
		}

		fmt.Printf("Comment added to %s: %s\n", issueKey, newComment.Body)
	},
}

// closeCmd transitions an issue to "Done", "Closed", or any target transitionName
var closeCmd = &cobra.Command{
	Use:   "close",
	Short: "Close (transition) an existing ticket to a given state (e.g. Done)",
	Long: `Attempt to transition the specified issue to a given state by name. 
By default, tries to move the ticket to "Done" unless --transitionName is set.`,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := getJiraClient()
		if err != nil {
			log.Fatalf("Error creating Jira client: %v", err)
		}

		if issueKey == "" {
			log.Fatal("Issue key is required (use --issueKey)")
		}
		if transitionName == "" {
			transitionName = "Done" // default transition name
		}
		if resolutionName == "" {
			resolutionName = "Done" // default resolution if none provided
		}

		// 1. Get available transitions
		transitions, resp, err := client.Issue.GetTransitions(issueKey)
		if err != nil {
			log.Fatalf("Error getting transitions for %s: %v\nResponse: %v", issueKey, err, resp)
		}

		// 2. Find the desired transition
		var desiredID string
		for _, t := range transitions {
			if strings.EqualFold(t.Name, transitionName) {
				desiredID = t.ID
				break
			}
		}

		if desiredID == "" {
			log.Fatalf("Transition '%s' not found for issue %s. Available transitions: %v",
				transitionName, issueKey, getTransitionNames(transitions))
		}

		// 3. Build transition payload with resolution
		transitionPayload := jira.CreateTransitionPayload{
			Transition: jira.TransitionPayload{
				ID: desiredID,
			},
			Fields: jira.TransitionPayloadFields{
				Resolution: &jira.Resolution{
					Name: resolutionName,
				},
			},
		}

		// 4. Execute the transition with the payload
		if _, err := client.Issue.DoTransitionWithPayload(issueKey, transitionPayload); err != nil {
			log.Fatalf("Error transitioning issue %s to '%s' with resolution '%s': %v",
				issueKey, transitionName, resolutionName, err)
		}

		fmt.Printf("Issue %s transitioned to '%s' (resolution: %s)\n", issueKey, transitionName, resolutionName)
	},
}

func init() {
	// Add subcommands to the root command
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(commentCmd)
	rootCmd.AddCommand(closeCmd)

	// Flags for listing tickets
	listCmd.Flags().StringVar(&projectKey, "projectKey", "", "Project key (e.g. TEST)")

	// Flags for creating tickets
	createCmd.Flags().StringVar(&projectKey, "projectKey", "", "Project key (e.g. TEST)")
	createCmd.Flags().StringVar(&summary, "summary", "", "Issue summary")
	createCmd.Flags().StringVar(&description, "description", "", "Issue description")
	createCmd.Flags().StringVar(&issueType, "issueType", "Task", "Issue type (e.g. Task, Bug, Story)")

	// Flags for updating tickets
	updateCmd.Flags().StringVar(&issueKey, "issueKey", "", "Issue key to update (e.g. TEST-123)")
	updateCmd.Flags().StringVar(&summary, "summary", "", "New summary for the issue")
	updateCmd.Flags().StringVar(&description, "description", "", "New description for the issue")

	// Flags for commenting
	commentCmd.Flags().StringVar(&issueKey, "issueKey", "", "Issue key to comment on (e.g. TEST-123)")
	commentCmd.Flags().StringVar(&commentBody, "body", "", "Comment body")

	// Flags for closing tickets
	closeCmd.Flags().StringVar(&issueKey, "issueKey", "", "Issue key to close (e.g. TEST-123)")
	closeCmd.Flags().StringVar(&transitionName, "transitionName", "", "Name of the desired transition (default: Done)")
}

// main executes the root command
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// getJiraClient constructs the Jira client using environment variables
func getJiraClient() (*jira.Client, error) {
	domain := os.Getenv("JIRA_DOMAIN")
	if domain == "" {
		return nil, fmt.Errorf("JIRA_DOMAIN environment variable is not set")
	}

	email := os.Getenv("JIRA_EMAIL")
	if email == "" {
		return nil, fmt.Errorf("JIRA_EMAIL environment variable is not set")
	}

	apiKey := os.Getenv("JIRA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("JIRA_API_KEY environment variable is not set")
	}

	tp := jira.BasicAuthTransport{
		Username: email,
		Password: apiKey,
	}
	return jira.NewClient(tp.Client(), fmt.Sprintf("https://%s", domain))
}

// getTransitionNames is a helper to format transition names for error messages
func getTransitionNames(transitions []jira.Transition) []string {
	var names []string
	for _, t := range transitions {
		names = append(names, t.Name)
	}
	return names
}
