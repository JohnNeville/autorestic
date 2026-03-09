package metadata

import (
	"regexp"
	"strings"
)

type ForgetLogMetadata struct {
	ToRepackBlobs        string
	ToRepackSize         string
	ThisRemovesBlobs     string
	ThisRemovesSize      string
	ToDeleteBlobs        string
	ToDeleteSize         string
	TotalPruneBlobs      string
	TotalPruneSize       string
	RemainingBlobs       string
	RemainingSize        string
	UnusedSizeAfterPrune string
}

var (
	reToRepack    = regexp.MustCompile(`(?i)^to repack:\s+(\d+) blobs / +([\d.]+ \w+)`)
	reThisRemoves = regexp.MustCompile(`(?i)^this removes:\s+(\d+) blobs / +([\d.]+ \w+)`)
	reToDelete    = regexp.MustCompile(`(?i)^to delete:\s+(\d+) blobs / +([\d.]+ \w+)`)
	reTotalPrune  = regexp.MustCompile(`(?i)^total prune:\s+(\d+) blobs / +([\d.]+ \w+)`)
	reRemaining   = regexp.MustCompile(`(?i)^remaining:\s+(\d+) blobs / +([\d.]+ \w+)`)
	reUnusedSize  = regexp.MustCompile(`(?i)^unused size after prune:\s+([\d.]+ \w+)`)
)

func extractPruneBlobsAndSize(re *regexp.Regexp, line string) (blobs, size string) {
	m := re.FindStringSubmatch(line)
	if len(m) >= 3 {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return "", ""
}

func ExtractMetadataFromForgetLog(log string) ForgetLogMetadata {
	var md ForgetLogMetadata
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case reToRepack.MatchString(line):
			md.ToRepackBlobs, md.ToRepackSize = extractPruneBlobsAndSize(reToRepack, line)
		case reThisRemoves.MatchString(line):
			md.ThisRemovesBlobs, md.ThisRemovesSize = extractPruneBlobsAndSize(reThisRemoves, line)
		case reToDelete.MatchString(line):
			md.ToDeleteBlobs, md.ToDeleteSize = extractPruneBlobsAndSize(reToDelete, line)
		case reTotalPrune.MatchString(line):
			md.TotalPruneBlobs, md.TotalPruneSize = extractPruneBlobsAndSize(reTotalPrune, line)
		case reRemaining.MatchString(line):
			md.RemainingBlobs, md.RemainingSize = extractPruneBlobsAndSize(reRemaining, line)
		case reUnusedSize.MatchString(line):
			m := reUnusedSize.FindStringSubmatch(line)
			if len(m) >= 2 {
				md.UnusedSizeAfterPrune = strings.TrimSpace(m[1])
			}
		}
	}
	return md
}

func MakeEnvFromForgetMetadata(md *ForgetLogMetadata) map[string]string {
	env := make(map[string]string)
	prefix := "AUTORESTIC_PRUNE_"
	env[prefix+"TO_REPACK_BLOBS"] = md.ToRepackBlobs
	env[prefix+"TO_REPACK_SIZE"] = md.ToRepackSize
	env[prefix+"THIS_REMOVES_BLOBS"] = md.ThisRemovesBlobs
	env[prefix+"THIS_REMOVES_SIZE"] = md.ThisRemovesSize
	env[prefix+"TO_DELETE_BLOBS"] = md.ToDeleteBlobs
	env[prefix+"TO_DELETE_SIZE"] = md.ToDeleteSize
	env[prefix+"TOTAL_BLOBS"] = md.TotalPruneBlobs
	env[prefix+"TOTAL_SIZE"] = md.TotalPruneSize
	env[prefix+"REMAINING_BLOBS"] = md.RemainingBlobs
	env[prefix+"REMAINING_SIZE"] = md.RemainingSize
	env[prefix+"UNUSED_SIZE"] = md.UnusedSizeAfterPrune
	return env
}
