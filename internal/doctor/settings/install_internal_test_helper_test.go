package settings

// CopyFileForTest runs the internal copyFile logic from tests.
func CopyFileForTest(src, dst string) error {
	return copyFile(src, dst)
}
