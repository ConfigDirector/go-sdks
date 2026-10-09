package evaluation

const (
	percentageSeed              = 0x397832987
	percentageWithoutIdentifier = 0.0
	percentageReachableBuckets  = 1000
	percentageBucketsPerPercent = 10
)

func hashedPercentage(configID string, identifier string) float64 {
	salt := []byte(identifier + "-" + configID)
	return float64(rapidhash(salt, percentageSeed)%percentageReachableBuckets) / percentageBucketsPerPercent
}

func assignedPercentage(configID string, identifier string) float64 {
	if identifier == "" {
		return percentageWithoutIdentifier
	}
	return hashedPercentage(configID, identifier)
}

func bucketFor(percentages []Percentage, assigned float64) (Percentage, bool) {
	total := 0.0
	for _, bucket := range percentages {
		if assigned < total+bucket.Percentage {
			return bucket, true
		}
		total += bucket.Percentage
	}
	return Percentage{}, false
}
