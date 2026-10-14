package evaluation

const (
	percentageSeed              = 0x397832987
	percentageWithoutIdentifier = 0.0
	percentageReachableBuckets  = 1000
	percentageBucketsPerPercent = 10
)

func hashedPercentage(configID string, context Context) float64 {
	return float64(rapidhash(percentageInput(configID, context), percentageSeed)%percentageReachableBuckets) / percentageBucketsPerPercent
}

func percentageInput(configID string, context Context) []byte {
	input := context.Identifier + "-" + configID
	if contextKind := context.effectiveContextKind(); contextKind != UserContextKind {
		input += ":" + contextKind
	}
	return []byte(input)
}

func assignedPercentage(configID string, context Context) float64 {
	if context.Identifier == "" {
		return percentageWithoutIdentifier
	}
	return hashedPercentage(configID, context)
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
