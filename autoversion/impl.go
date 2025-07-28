package autoversion

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
)

const helpText = "I look for a version-line like this:\n# some prefix v0.1 (2020-09-06) (45142f9a49d45793)"

var reVersionLine = regexp.MustCompile(`^(.*) v([\d\.]+) \(([\d-]+)\) \(([\da-z]+)\)([\ \-\>\<]*)$`)

func autoversionFile(path string) error {
	output := ""
	versionLineCount := 0
	versionLineIndex := 0
	versionLine := ""

	var (
		rPrefix   string
		rVersion  string
		rChecksum string
		rEnd      string
	)

	readFile := func() error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		i := 0
		scanner := bufio.NewScanner(bufio.NewReader(f))
		for scanner.Scan() {
			line := scanner.Text()

			matches := reVersionLine.FindStringSubmatch(line)
			if len(matches) == 0 {
				output += line
				output += "\n"
				i++
				continue
			}

			if versionLine != "" {
				return fmt.Errorf("we already found more than one version-line matches on lines %d and %d", versionLineCount, i)
			}

			rPrefix = matches[1]
			rVersion = matches[2]
			// ignore matches[3] (date)
			rChecksum = matches[4]
			rEnd = matches[5]

			versionLineCount = i
			versionLineIndex = len(output)
			versionLine = line
			i++
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	}
	err := readFile()
	if err != nil {
		return err
	}

	if versionLine == "" {
		// there's no version line
		fmt.Printf("autoversion: no-version %s\n\n%s\n", path, helpText)
		return nil
	}

	checksum := fmt.Sprintf("%x", xxhash.Sum64String(output)) // xxhash

	if rChecksum == checksum {
		// checksum matches, nothing to be done
		fmt.Printf("autoversion: up-to-date %s\n", path)
		return nil
	}

	var newVersionLine string
	{ // create newVersionLine
		versionArr := strings.Split(rVersion, ".")

		lastVersionIndex := len(versionArr) - 1
		lastVersion, err := strconv.ParseInt(versionArr[lastVersionIndex], 10, 64)
		if err != nil {
			return err
		}

		versionArr[lastVersionIndex] = fmt.Sprintf("%d", lastVersion+1)
		version := strings.Join(versionArr, ".")

		today := time.Now().Format("2006-01-02")

		newVersionLine = fmt.Sprintf("%s v%s (%s) (%s)%s\n",
			rPrefix, version, today, checksum, rEnd,
		)
	}

	result := output[:versionLineIndex]
	result += newVersionLine
	if len(output) > versionLineIndex { // there's more
		result += output[versionLineIndex:]
	}

	err = os.WriteFile(path, []byte(result), 0644)
	if err != nil {
		return err
	}

	fmt.Printf("autoversion:    updated %s\n", path)
	return nil
}
