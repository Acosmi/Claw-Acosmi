package gateway

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

func captureGatewayConfigRollback(loader *config.ConfigLoader) (*GatewayConfigRollback, error) {
	if loader == nil {
		return nil, nil
	}

	configPath := strings.TrimSpace(loader.ConfigPath())
	if configPath == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(configPath)
	if err == nil {
		return &GatewayConfigRollback{
			ConfigPath:  configPath,
			PreviousRaw: append([]byte(nil), raw...),
		}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &GatewayConfigRollback{
			ConfigPath:       configPath,
			DeleteOnRollback: true,
		}, nil
	}
	return nil, fmt.Errorf("capture config rollback: %w", err)
}

func cloneGatewayConfigRollback(rollback *GatewayConfigRollback) *GatewayConfigRollback {
	if rollback == nil {
		return nil
	}
	cloned := &GatewayConfigRollback{
		ConfigPath:       rollback.ConfigPath,
		DeleteOnRollback: rollback.DeleteOnRollback,
	}
	if len(rollback.PreviousRaw) > 0 {
		cloned.PreviousRaw = append([]byte(nil), rollback.PreviousRaw...)
	}
	return cloned
}

func restoreGatewayConfigRollback(rollback *GatewayConfigRollback) error {
	if rollback == nil || strings.TrimSpace(rollback.ConfigPath) == "" {
		return nil
	}

	if rollback.DeleteOnRollback {
		if err := os.Remove(rollback.ConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove config rollback target: %w", err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(rollback.ConfigPath), 0o700); err != nil {
		return fmt.Errorf("mkdir config rollback dir: %w", err)
	}
	if err := os.WriteFile(rollback.ConfigPath, rollback.PreviousRaw, 0o600); err != nil {
		return fmt.Errorf("restore config rollback target: %w", err)
	}
	return nil
}

func rollbackGatewayConfigOnError(loader *config.ConfigLoader, rollback *GatewayConfigRollback, err error) error {
	if rollback == nil {
		return err
	}
	if restoreErr := restoreGatewayConfigRollback(rollback); restoreErr != nil {
		return fmt.Errorf("%v; rollback restore failed: %w", err, restoreErr)
	}
	if loader != nil {
		loader.ClearCache()
	}
	return err
}
