package convo

func Key(channelID, userID string) string {
	return channelID + "|" + userID
}
