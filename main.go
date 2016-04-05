// Creator: Göran Gustafsson (gustafsson.g at gmail.com)
// License: BSD 3-Clause

// sredd - s(ub)redd(it)
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

const (
	appName     = "sredd"
	appLongName = "s(ub)redd(it)"
	appVersion  = "0.4"
)

// config is a global variable containing current user and runtime settings.
var config options

// options is a struct that defines all configuration values.
type options struct {
	Command        string
	CommandArgs    []string
	FilterComments bool
	ProgramPath    string
	Subreddits     []string
}

// response is a struct that defines the expected JSON response from Reddit.
type response struct {
	Data struct {
		Children []struct {
			Data struct {
				URL string
			}
		}
	}
}

// checkNew reads in old URL's from log file (if such a file exists), creates a
// new log file containing new URL's, and compares new and old URL lists.
// Returns list of all new URL's.
func checkNew(name string, urls []string) (newURLs []string, err error) {
	log := fmt.Sprintf("%s/r_%s.log", config.ProgramPath, name)
	oldURLs := make([]string, 0, 25)
	// Read log file and add URLs to array.
	if _, err := os.Stat(log); err == nil {
		file, err := os.Open(log)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			oldURLs = append(oldURLs, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		file.Close()
	}

	// Write all new URLs to log file.
	file, err := os.Create(log)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, url := range urls {
		writer.WriteString(fmt.Sprintf("%s\n", url))
		if err != nil {
			return nil, err
		}
	}
	err = writer.Flush()
	if err != nil {
		return nil, err
	}
	// Compare list of new and old URL's.
	var dup int
	for _, url := range urls {
		dup = 0
		for _, oldURL := range oldURLs {
			if url == oldURL {
				dup = 1
			}
		}
		if dup == 0 {
			newURLs = append(newURLs, url)
		}
	}
	return newURLs, nil
}

// checkSub checks specific Subreddit for new posts. Returns list of URL's.
func checkSub(name string) (urls []string, err error) {
	url := fmt.Sprintf("https://reddit.com/r/%s.json", name)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// The most popular HTTP User-Agent as of 2016-03-28.
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/48.0.2564.116 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	sub := new(response)
	err = json.NewDecoder(resp.Body).Decode(&sub)
	if err != nil {
		return nil, err
	}
	// Loop over all Subreddits posts.
	for _, item := range sub.Data.Children {
		itemURL := item.Data.URL
		// Filter discussion threads if FilterComments is disabled in config.
		if config.FilterComments && strings.Contains(itemURL, "/comments/") {
			continue
		}
		// Make sure items always starts with either http:// or https://.
		match, _ := regexp.MatchString("^https?://", itemURL)
		if match == false {
			continue
		}
		urls = append(urls, itemURL)
	}
	return urls, nil
}

// execCommand prints out list of URL's and executes user specified command.
func execCommand(urls []string) (err error) {
	for _, url := range urls {
		fmt.Printf("URL: %s\n", url)
	}
	// cmd contains the main command, e.g. "open".
	cmd := config.Command
	// args contains all arguments used with cmd, e.g. "-a Safari <URL1> ...".
	args := append(config.CommandArgs, urls...)
	err = exec.Command(cmd, args...).Run()
	if err != nil {
		return err
	}
	return nil
}

// readConfig reads JSON config file and set values in struct variable config.
func readConfig() (err error) {
	usr, err := user.Current()
	if err != nil {
		return err
	}
	// Location of config and log files, e.g. "~/.sredd/config.json".
	config.ProgramPath = fmt.Sprintf("%s/.%s", usr.HomeDir, appName)
	path := fmt.Sprintf("%s/config.json", config.ProgramPath)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return err
	}
	if config.Command == "" {
		return errors.New("option 'Command' not set")
	}
	if len(config.Subreddits) == 0 {
		return errors.New("option 'Subreddits' not set")
	}
	return nil
}

// usage prints out information about how to use the program.
func usage() {
	info := `
Options:
    -h, --help       Display this help text
    -v, --version    Display version information
`
	fmt.Printf("Usage: %s [OPTION]\n", appName)
	fmt.Printf("%s", info)
}

// version prints out various information about the program.
func version() {
	info := `
Web: https://github.com/ggustafsson/sredd
Git: https://github.com/ggustafsson/sredd.git

Written by Göran Gustafsson <gustafsson.g@gmail.com>
Released under the BSD 3-Clause license
`
	fmt.Printf("%s - %s, version %s\n", appName, appLongName, appVersion)
	fmt.Printf("%s", info)
}

func init() {
	// Only accept one single argument, or none at all.
	if len(os.Args[1:]) == 1 {
		switch os.Args[1] {
		case "-h", "--help":
			usage()
		case "-v", "--version":
			version()
		default:
			usage()
			os.Exit(1)
		}
		os.Exit(0)
	} else if len(os.Args[1:]) >= 2 {
		usage()
		os.Exit(1)
	}

	// Load configuration file so program can run.
	err := readConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	for index, name := range config.Subreddits {
		fmt.Printf("Checking r/%s for new posts...\n", name)
		// Check subreddit and return all URL's.
		urls, err := checkSub(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Subreddit error: %v\n", err)
			os.Exit(1)
		}
		// Check which URL's are new compared to last run.
		newURLs, err := checkNew(name, urls)
		if err != nil {
			fmt.Fprintf(os.Stderr, "New posts error: %v\n", err)
			os.Exit(1)
		}
		if len(newURLs) == 0 {
			fmt.Println("No new posts found!")
			// Only print newline if there are subreddits left.
			if index != len(config.Subreddits)-1 {
				fmt.Println()
			}
			continue
		}
		err = execCommand(newURLs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Command error: %v\n", err)
			os.Exit(1)
		}
		// Only wait for input if there are subreddits left.
		if index == len(config.Subreddits)-1 {
			break
		}
		fmt.Printf("Press 'Return' key when ready to continue...")
		terminal.ReadPassword(int(syscall.Stdin))
		fmt.Printf("\n\n")
	}
}
