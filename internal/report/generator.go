package report

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"scan2html/internal/common"
	"scan2html/internal/epss"
	"scan2html/internal/logger"
	"strings"
	"time"
)

func GenerateHtmlReport(pluginFlags common.Flags, version string) error {
	logger.Logger.Infof("GenerateHtmlReport: %v", pluginFlags)
	defer os.Remove(common.GetScan2htmlTempReportPath())

	baseDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatalf("Failed to determine base directory: %v", err)
	}

	reportName := pluginFlags["--output"]
	_, withEpss := pluginFlags["--with-epss"]
	reportTitle := pluginFlags["--report-title"]
	// Log input parameters for clarity
	logger.Logger.Infof("Base Directory: %s\n", baseDir)
	logger.Logger.Infof("With EPSS: %t\n", withEpss)
	logger.Logger.Infof("Report Title: %s\n", reportTitle)
	logger.Logger.Infof("Report Name: %s\n", reportName)

	// Generate a unique report name if necessary
	reportName = generateReportName(reportName)

	// Append the report template to the report file
	templateContent, _ := common.ReadPluginFile("report_template.html")
	if err := os.WriteFile(reportName, templateContent, 0644); err != nil {
		return fmt.Errorf("could not create report file %s: %v", reportName, err)
	}

	err = replaceTextByText(reportName, "TEMP_APP_VERSION", version)
	if err != nil {
		return fmt.Errorf("failed to replace report title in %s: %v", reportName, err)
	}

	// Replace placeholders with actual content in the report file
	err = replaceTextByText(reportName, "{REPORT_TITLE:\"Temp Trivy Report\"}", fmt.Sprintf("{REPORT_TITLE:'%s'}", reportTitle))
	if err != nil {
		return fmt.Errorf("failed to replace report title in %s: %v", reportName, err)
	}
	err = replaceTextByFile(reportName, "{TEMP_RESULTS:0}", common.GetScan2htmlTempReportPath())
	if err != nil {
		return fmt.Errorf("failed to replace temp results in %s: %v", reportName, err)
	}

	// Handle EPSS data if enabled
	if withEpss {
		logger.Logger.Infoln("EPSS enabled!")
		var epssDataFile, err = epss.PrepareEpssData()
		if err != nil {
			return fmt.Errorf("failed to prepare EPSS data: %v", err)
		}

		// replaceTextByFile "$report_name" "\"TEMP_EPSS_DATA\"" "$epss_data"
		if err := replaceTextByFile(reportName, "\"TEMP_EPSS_DATA\"", epssDataFile); err != nil {
			return fmt.Errorf("failed to replace EPSS data in %s: %v", reportName, err)
		}

		logger.Logger.Infoln("EPSS data imported!")

		// Schedule deletion of the EPSS data file upon function exit
		defer os.Remove(epssDataFile)
	}

	logger.Logger.Infof("%s has been created successfully!\n", reportName)
	return nil
}

// replaceTextByText replaces occurrences of search_text in the input file with replace_content.
func replaceTextByText(inputFile, searchText, replaceContent string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("could not open file %s: %v", inputFile, err)
	}
	defer file.Close()

	tempFile, err := os.CreateTemp("", "modified_")
	if err != nil {
		return fmt.Errorf("could not create temp file: %v", err)
	}
	defer tempFile.Close()

	reader := bufio.NewReader(file)
	writer := bufio.NewWriter(tempFile)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading file: %v", err)
		}

		if strings.Contains(line, searchText) {
			before := strings.SplitN(line, searchText, 2)[0]
			after := strings.SplitN(line, searchText, 2)[1]
			replacedLine := fmt.Sprintf("%s%s%s", before, replaceContent, after)
			_, _ = writer.WriteString(replacedLine)
		} else {
			_, _ = writer.WriteString(line)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("error writing to temp file: %v", err)
	}

	
	return copyAndRemove(tempFile.Name(), inputFile)
}

func copyAndRemove(src, dst string) error {
    // Open the source file
    sourceFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer sourceFile.Close()

    // Create the destination file
    destFile, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer destFile.Close()

    // Copy the contents
    if _, err := io.Copy(destFile, sourceFile); err != nil {
        return err
    }

    // Close files before removal
    sourceFile.Close()
    destFile.Close()

    // Remove the source file
    return os.Remove(src)
}

// replaceTextByFile replaces occurrences of search_text in the input file with content from replace_file.
func replaceTextByFile(inputFile, searchText, replaceFile string) error {
	replaceContent, err := os.ReadFile(replaceFile)
	if err != nil {
		return fmt.Errorf("could not read file %s: %v", replaceFile, err)
	}
	return replaceTextByText(inputFile, searchText, string(replaceContent))
}

// generateReportName creates a unique report name based on timestamp if the file already exists.
func generateReportName(reportName string) string {
	if _, err := os.Stat(reportName); os.IsNotExist(err) {
		return reportName // File doesn't exist, return the original name
	}

	// Generate a new report name with a timestamp
	timestamp := time.Now().Format("2006_01_02_15_04_05_06")
	newReportName := strings.Replace(reportName, ".html", fmt.Sprintf("(%s).html", timestamp), 1)
	logger.Logger.Infof("File %s already exists. Using %s instead.\n", reportName, newReportName)

	return newReportName
}

func CombineReports(pluginFlags common.Flags) error {
	logger.Logger.Infoln("Function: combineReports")
	from := pluginFlags["--from"]
	resultFiles := strings.Split(from, ",")
	logger.Logger.Infof("From resultFiles: %v\n", resultFiles)

	var resultFileContents []string

	// Iterate through each file and combine their contents
	for _, file := range resultFiles {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				logger.Logger.Infof("File %s does not exist.\n", file)
				return fmt.Errorf("file %s does not exist", file)
			}
			return fmt.Errorf("failed to check file %s: %w", file, err)
		}

		logger.Logger.Infof("Reading contents of %s:\n", file)
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", file, err)
		}

		resultFileContents = append(resultFileContents, string(content))
	}

	// Combine contents into a single string
	combinedContent := strings.Join(resultFileContents, ",")

	// Write the combined contents to the result file
	if err := os.WriteFile(common.GetScan2htmlTempReportPath(), []byte(combinedContent), 0644); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", common.GetScan2htmlTempReportPath(), err)
	}

	logger.Logger.Infof("Content written to %s\n", common.GetScan2htmlTempReportPath())
	return nil
}
