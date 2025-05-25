package utils

func GenerateToken(userID int) (string, error) {
	// TODO: Generate JWT
	return "secret_token", nil
}

func ValidateToken(token string) (int, error) {
	// TODO: Parse JWT
	return 1, nil
}
