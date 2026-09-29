package internal

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/JohnNeville/autorestic/internal/lock"
	"github.com/JohnNeville/autorestic/internal/metadata"
	"github.com/robfig/cron"
)

type LocationType string

const (
	TypeLocal  LocationType = "local"
	TypeVolume LocationType = "volume"
)

type HookArray = []string

type LocationForgetOption string

const (
	LocationForgetYes   LocationForgetOption = "yes"
	LocationForgetNo    LocationForgetOption = "no"
	LocationForgetPrune LocationForgetOption = "prune"
)

type Hooks struct {
	Dir         string    `mapstructure:"dir" yaml:"dir"`
	PreValidate HookArray `mapstructure:"prevalidate,omitempty" yaml:"prevalidate,omitempty"`
	Before      HookArray `mapstructure:"before,omitempty" yaml:"before,omitempty"`
	After       HookArray `mapstructure:"after,omitempty" yaml:"after,omitempty"`
	Success     HookArray `mapstructure:"success,omitempty" yaml:"success,omitempty"`
	Failure     HookArray `mapstructure:"failure,omitempty" yaml:"failure,omitempty"`
}

type LocationCopy = map[string][]string

type Location struct {
	name         string               `mapstructure:",omitempty" yaml:",omitempty"`
	From         []string             `mapstructure:"from,omitempty" yaml:"from,omitempty"`
	Type         string               `mapstructure:"type,omitempty" yaml:"type,omitempty"`
	To           []string             `mapstructure:"to,omitempty" yaml:"to,omitempty"`
	Hooks        Hooks                `mapstructure:"hooks,omitempty" yaml:"hooks,omitempty"`
	ForgetHooks  Hooks                `mapstructure:"forget-hooks,omitempty" yaml:"forget-hooks,omitempty"`
	VerifyHooks  Hooks                `mapstructure:"verify-hooks,omitempty" yaml:"verify-hooks,omitempty"`
	Cron         string               `mapstructure:"cron,omitempty" yaml:"cron,omitempty"`
	VerifyCron   string               `mapstructure:"verify-cron,omitempty" yaml:"verify-cron,omitempty"`
	Options      Options              `mapstructure:"options,omitempty" yaml:"options,omitempty"`
	ForgetOption LocationForgetOption `mapstructure:"forget,omitempty" yaml:"forget,omitempty"`
	CopyOption   LocationCopy         `mapstructure:"copy,omitempty" yaml:"copy,omitempty"`
	Envs         []string             `mapstructure:"envs,omitempty" yaml:"envs,omitempty"`
}

func GetLocation(name string) (Location, bool) {
	l, ok := GetConfig().Locations[name]
	l.name = name
	return l, ok
}

// parseEnvs converts the Envs slice (in key=value format) to a map
func (l Location) parseEnvs() map[string]string {
	envMap := make(map[string]string)
	for _, env := range l.Envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	return envMap
}

func (l Location) validate() error {
	if len(l.From) == 0 {
		return fmt.Errorf(`Location "%s" is missing "from" key`, l.name)
	}
	t, err := l.getType()
	if err != nil {
		return err
	}
	switch t {
	case TypeLocal:
		for _, path := range l.From {
			if from, err := GetPathRelativeToConfig(path); err != nil {
				return err
			} else {
				if _, err := os.Stat(from); err != nil {
					return err
				}
			}
		}
	case TypeVolume:
		if len(l.From) > 1 {
			return fmt.Errorf(`location "%s" has more than one docker volume`, l.name)
		}
	}

	if len(l.To) == 0 {
		return fmt.Errorf(`location "%s" has no "to" targets`, l.name)
	}
	// Check if backends are all valid
	for _, to := range l.To {
		_, ok := GetBackend(to)
		if !ok {
			return fmt.Errorf(`location "%s" has an invalid backend "%s"`, l.name, to)
		}
	}

	// Check copy option
	for copyFrom, copyTo := range l.CopyOption {
		if _, ok := GetBackend(copyFrom); !ok {
			return fmt.Errorf(`location "%s" has an invalid backend "%s" in copy option`, l.name, copyFrom)
		}
		if !ArrayContains(l.To, copyFrom) {
			return fmt.Errorf(`location "%s" has an invalid copy from "%s"`, l.name, copyFrom)
		}
		for _, copyToTarget := range copyTo {
			if _, ok := GetBackend(copyToTarget); !ok {
				return fmt.Errorf(`location "%s" has an invalid backend "%s" in copy option`, l.name, copyToTarget)
			}
			if ArrayContains(l.To, copyToTarget) {
				return fmt.Errorf(`location "%s" cannot copy to "%s" as it's already a target`, l.name, copyToTarget)
			}
		}
	}

	// Check if forget type is correct
	if l.ForgetOption != "" {
		if l.ForgetOption != LocationForgetYes && l.ForgetOption != LocationForgetNo && l.ForgetOption != LocationForgetPrune {
			return fmt.Errorf("invalid value for forget option: %s", l.ForgetOption)
		}
	}
	return nil
}

func (l Location) ExecuteHooks(commands []string, options ExecuteOptions) error {
	if len(commands) == 0 {
		return nil
	}
	if l.Hooks.Dir != "" {
		if dir, err := GetPathRelativeToConfig(l.Hooks.Dir); err != nil {
			return err
		} else {
			options.Dir = dir
		}
	}
	colors.Secondary.Println("\nRunning hooks")
	for _, command := range commands {
		colors.Body.Println("> " + command)
		_, out, err := ExecuteCommand(options, "-c", command)
		if err != nil {
			colors.Error.Println(out)
			return err
		}
	}
	colors.Body.Println("")
	return nil
}

func (l Location) getType() (LocationType, error) {
	t := strings.ToLower(l.Type)
	if t == "" || t == "local" {
		return TypeLocal, nil
	} else if t == "volume" {
		return TypeVolume, nil
	}
	return "", fmt.Errorf("invalid location type \"%s\"", l.Type)
}

func buildTag(parts ...string) string {
	parts = append([]string{"ar"}, parts...)
	return strings.Join(parts, ":")
}

func (l Location) getLocationTags() string {
	return buildTag("location", l.name)
}

func (l Location) Backup(cron bool, dry bool, specificBackend string) []error {
	var errors []error
	var backends []string
	colors.PrimaryPrint("  Backing up location \"%s\"  ", l.name)
	t, err := l.getType()
	if err != nil {
		errors = append(errors, err)
		return errors
	}
	cwd, _ := GetPathRelativeToConfig(".")
	// User-defined envs are applied first so system vars always take priority
	envs := l.parseEnvs()
	envs["AUTORESTIC_LOCATION"] = l.name
	options := ExecuteOptions{
		Command: "bash",
		Dir:     cwd,
		Envs:    envs,
	}

	// Hooks before location validation
	if err := l.ExecuteHooks(l.Hooks.PreValidate, options); err != nil {
		errors = append(errors, err)
		goto after
	}

	if err := l.validate(); err != nil {
		errors = append(errors, err)
		goto after
	}

	// Hooks after location validation
	if err := l.ExecuteHooks(l.Hooks.Before, options); err != nil {
		errors = append(errors, err)
		goto after
	}

	// Backup
	if specificBackend == "" {
		backends = l.To
	} else {
		if l.hasBackend(specificBackend) {
			backends = []string{specificBackend}
		} else {
			errors = append(errors, fmt.Errorf("backup location \"%s\" has no backend \"%s\"", l.name, specificBackend))
			return errors
		}
	}
	for i, to := range backends {
		backend, _ := GetBackend(to)
		colors.Secondary.Printf("Backend: %s\n", backend.name)
		env, err := backend.getEnv()
		if err != nil {
			errors = append(errors, err)
			continue
		}

		cmd := []string{"backup"}
		cmd = append(cmd, combineAllOptions("backup", l, backend)...)
		if cron {
			cmd = append(cmd, "--tag", buildTag("cron"))
		}
		if dry {
			cmd = append(cmd, "--dry-run")
		}
		cmd = append(cmd, "--tag", l.getLocationTags())
		backupOptions := ExecuteOptions{
			Envs: env,
		}

		var code int = 0
		var out string
		switch t {
		case TypeLocal:
			for _, from := range l.From {
				path, err := GetPathRelativeToConfig(from)
				if err != nil {
					errors = append(errors, err)
					goto after
				}
				cmd = append(cmd, path)
			}
			code, out, err = ExecuteResticCommand(backupOptions, cmd...)
		case TypeVolume:
			ok := CheckIfVolumeExists(l.From[0])
			if !ok {
				errors = append(errors, fmt.Errorf("volume \"%s\" does not exist", l.From[0]))
				continue
			}
			cmd = append(cmd, "/data")
			code, out, err = backend.ExecDocker(l, cmd)
		}

		// Extract metadata
		md := metadata.ExtractMetadataFromBackupLog(out)
		md.ExitCode = fmt.Sprint(code)
		mdEnv := metadata.MakeEnvFromMetadata(&md)
		for k, v := range mdEnv {
			options.Envs[k+"_"+fmt.Sprint(i)] = v
			options.Envs[k+"_"+strings.ToUpper(backend.name)] = v
		}

		// If error save it and continue
		if err != nil {
			colors.Error.Println(out)
			errors = append(errors, fmt.Errorf("%s@%s:\n%s%s", l.name, backend.name, out, err))
			continue
		}

		// Copy
		if md.SnapshotID != "" {
			for copyFrom, copyTo := range l.CopyOption {
				b1, _ := GetBackend(copyFrom)
				for _, copyToTarget := range copyTo {
					b2, _ := GetBackend(copyToTarget)
					colors.Secondary.Println("Copying " + copyFrom + " → " + copyToTarget)
					env, _ := b1.getEnv()
					env2, _ := b2.getEnv()
					// Add the second repo to the env with a "2" suffix
					for k, v := range env2 {
						env[k+"2"] = v
					}
					_, _, err := ExecuteResticCommand(ExecuteOptions{
						Envs: env,
					}, "copy", md.SnapshotID)

					if err != nil {
						errors = append(errors, err)
					}
				}
			}
		}
	}

	// After backup hooks
	if err := l.ExecuteHooks(l.Hooks.After, options); err != nil {
		errors = append(errors, err)
	}

after:
	// Success/failure hooks
	var commands []string
	var isSuccess = len(errors) == 0
	if isSuccess {
		commands = l.Hooks.Success
	} else {
		commands = l.Hooks.Failure
		errMsgs := make([]string, len(errors))
		for i, e := range errors {
			errMsgs[i] = e.Error()
		}
		options.Envs["AUTORESTIC_FAILURE_REASON"] = strings.Join(errMsgs, "\n")
	}
	if err := l.ExecuteHooks(commands, options); err != nil {
		errors = append(errors, err)
	}

	// Forget and optionally prune
	if isSuccess && l.ForgetOption != "" && l.ForgetOption != LocationForgetNo {
		err := l.Forget(l.ForgetOption == LocationForgetPrune, false)
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) == 0 {
		colors.Success.Println("Done")
	}
	return errors
}

func (l Location) Forget(prune bool, dry bool) error {
	colors.PrimaryPrint("Forgetting for location \"%s\"", l.name)

	cwd, _ := GetPathRelativeToConfig(".")
	// User-defined envs are applied first so system vars always take priority
	hookEnvs := l.parseEnvs()
	hookEnvs["AUTORESTIC_LOCATION"] = l.name
	hookOptions := ExecuteOptions{
		Command: "bash",
		Dir:     cwd,
		Envs:    hookEnvs,
	}

	if err := l.ExecuteHooks(l.ForgetHooks.Before, hookOptions); err != nil {
		hookOptions.Envs["AUTORESTIC_FAILURE_REASON"] = err.Error()
		if hookErr := l.ExecuteHooks(l.ForgetHooks.Failure, hookOptions); hookErr != nil {
			colors.Error.Println(hookErr)
		}
		return err
	}

	var forgetErr error
	backendsToForget := l.To
	for _, copyBackends := range l.CopyOption {
		backendsToForget = append(backendsToForget, copyBackends...)
	}

	for i, to := range backendsToForget {
		backend, _ := GetBackend(to)
		colors.Secondary.Printf("For backend \"%s\"\n", backend.name)
		env, err := backend.getEnv()
		if err != nil {
			forgetErr = err
			continue
		}
		options := ExecuteOptions{
			Envs: env,
		}
		cmd := []string{"forget", "--tag", l.getLocationTags()}
		if prune {
			cmd = append(cmd, "--prune")
		}
		if dry {
			cmd = append(cmd, "--dry-run")
		}
		cmd = append(cmd, combineAllOptions("forget", l, backend)...)
		_, out, err := ExecuteResticCommand(options, cmd...)
		if err != nil {
			forgetErr = err
		} else {
			// Extract and expose prune stats for hooks on success
			md := metadata.ExtractMetadataFromForgetLog(out)
			mdEnv := metadata.MakeEnvFromForgetMetadata(&md)
			for k, v := range mdEnv {
				hookOptions.Envs[k+"_"+fmt.Sprint(i)] = v
				hookOptions.Envs[k+"_"+strings.ToUpper(backend.name)] = v
			}
		}
	}

	if err := l.ExecuteHooks(l.ForgetHooks.After, hookOptions); err != nil {
		colors.Error.Println(err)
		if forgetErr == nil {
			forgetErr = err
		}
	}

	if forgetErr != nil {
		hookOptions.Envs["AUTORESTIC_FAILURE_REASON"] = forgetErr.Error()
		if err := l.ExecuteHooks(l.ForgetHooks.Failure, hookOptions); err != nil {
			colors.Error.Println(err)
		}
		return forgetErr
	}

	if err := l.ExecuteHooks(l.ForgetHooks.Success, hookOptions); err != nil {
		return err
	}

	colors.Success.Println("Done")
	return nil
}

// Verify runs "restic check" against every backend the location writes to,
// including copy targets. Flags come from the "check" options key.
func (l Location) Verify() error {
	colors.PrimaryPrint("Verifying for location \"%s\"", l.name)

	cwd, _ := GetPathRelativeToConfig(".")
	// User-defined envs are applied first so system vars always take priority
	hookEnvs := l.parseEnvs()
	hookEnvs["AUTORESTIC_LOCATION"] = l.name
	hookOptions := ExecuteOptions{
		Command: "bash",
		Dir:     cwd,
		Envs:    hookEnvs,
	}

	if err := l.ExecuteHooks(l.VerifyHooks.Before, hookOptions); err != nil {
		hookOptions.Envs["AUTORESTIC_FAILURE_REASON"] = err.Error()
		if hookErr := l.ExecuteHooks(l.VerifyHooks.Failure, hookOptions); hookErr != nil {
			colors.Error.Println(hookErr)
		}
		return err
	}

	backendsToVerify := append([]string{}, l.To...)
	for _, copyBackends := range l.CopyOption {
		backendsToVerify = append(backendsToVerify, copyBackends...)
	}

	var verifyErrs []error
	for _, to := range backendsToVerify {
		backend, _ := GetBackend(to)
		colors.Secondary.Printf("For backend \"%s\"\n", backend.name)
		env, err := backend.getEnv()
		if err != nil {
			verifyErrs = append(verifyErrs, fmt.Errorf("%s@%s: %w", l.name, backend.name, err))
			continue
		}
		cmd := append([]string{"check"}, combineAllOptions("check", l, backend)...)
		_, out, err := ExecuteResticCommand(ExecuteOptions{Envs: env}, cmd...)
		if err != nil {
			verifyErrs = append(verifyErrs, fmt.Errorf("%s@%s:\n%s%s", l.name, backend.name, out, err))
		}
	}

	if err := l.ExecuteHooks(l.VerifyHooks.After, hookOptions); err != nil {
		verifyErrs = append(verifyErrs, err)
	}

	if len(verifyErrs) > 0 {
		verifyErr := errors.Join(verifyErrs...)
		hookOptions.Envs["AUTORESTIC_FAILURE_REASON"] = verifyErr.Error()
		if err := l.ExecuteHooks(l.VerifyHooks.Failure, hookOptions); err != nil {
			colors.Error.Println(err)
		}
		return verifyErr
	}

	if err := l.ExecuteHooks(l.VerifyHooks.Success, hookOptions); err != nil {
		return err
	}

	colors.Success.Println("Done")
	return nil
}

func (l Location) hasBackend(backend string) bool {
	for _, b := range l.To {
		if b == backend {
			return true
		}
	}
	return false
}

func buildRestoreCommand(l Location, to string, snapshot string, options []string) []string {
	base := []string{"restore", "--target", to, "--tag", l.getLocationTags(), snapshot}
	base = append(base, options...)
	return base
}

func (l Location) Restore(to, from string, force bool, snapshot string, options []string) error {
	if from == "" {
		from = l.To[0]
	} else if !l.hasBackend(from) {
		return fmt.Errorf("invalid backend: \"%s\"", from)
	}

	if snapshot == "" {
		snapshot = "latest"
	}

	colors.PrimaryPrint("Restoring location \"%s\"", l.name)
	backend, _ := GetBackend(from)
	colors.Secondary.Printf("Restoring %s@%s → %s\n", snapshot, backend.name, to)

	t, err := l.getType()
	if err != nil {
		return err
	}
	switch t {
	case TypeLocal:
		to, err = filepath.Abs(to)
		if err != nil {
			return err
		}
		// Check if target is empty
		if !force {
			notEmptyError := fmt.Errorf("target %s is not empty", to)
			_, err = os.Stat(to)
			if err == nil {
				files, err := ioutil.ReadDir(to)
				if err != nil {
					return err
				}
				if len(files) > 0 {
					return notEmptyError
				}
			} else {
				if !os.IsNotExist(err) {
					return err
				}
			}
		}
		err = backend.Exec(buildRestoreCommand(l, to, snapshot, options))
	case TypeVolume:
		_, _, err = backend.ExecDocker(l, buildRestoreCommand(l, "/", snapshot, options))
	}
	if err != nil {
		return err
	}
	colors.Success.Println("Done")
	return nil
}

func (l Location) RunCron() error {
	var errs []error

	if l.Cron != "" {
		due, err := isDue(l.Cron, lock.GetCron(l.name))
		if err != nil {
			return err
		}
		if due {
			lock.SetCron(l.name, time.Now().Unix())
			if backupErrs := l.Backup(true, false, ""); len(backupErrs) > 0 {
				errs = append(errs, fmt.Errorf("Failed to backup location \"%s\":\n%w", l.name, errors.Join(backupErrs...)))
			}
		} else if !flags.CRON_LEAN {
			colors.Body.Printf("Skipping \"%s\", not due yet.\n", l.name)
		}
	}

	if l.VerifyCron != "" {
		last := lock.GetVerifyCron(l.name)
		if last == 0 {
			// First sighting: start the schedule now instead of running a
			// potentially hours-long check on the very next cron tick.
			lock.SetVerifyCron(l.name, time.Now().Unix())
			if !flags.CRON_LEAN {
				colors.Body.Printf("Scheduling verify of \"%s\" from now on.\n", l.name)
			}
			return errors.Join(errs...)
		}
		due, err := isDue(l.VerifyCron, last)
		if err != nil {
			return err
		}
		if due {
			lock.SetVerifyCron(l.name, time.Now().Unix())
			if err := l.Verify(); err != nil {
				errs = append(errs, fmt.Errorf("Failed to verify location \"%s\":\n%w", l.name, err))
			}
		} else if !flags.CRON_LEAN {
			colors.Body.Printf("Skipping verify of \"%s\", not due yet.\n", l.name)
		}
	}

	return errors.Join(errs...)
}

// isDue reports whether a cron expression has fired since the unix time last.
func isDue(expr string, last int64) (bool, error) {
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return false, err
	}
	return time.Now().After(schedule.Next(time.Unix(last, 0))), nil
}
