package config

// Export* symbols are test hooks (this file is not compiled into non-test builds).
// Names avoid the "Test*" prefix so the Go test harness does not treat them as tests.

func ExportNormalize(cfg *Config, baseDir string) { normalize(cfg, baseDir) }

func ExportValidate(cfg Config) error { return validate(cfg) }
