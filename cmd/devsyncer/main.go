/* SPDX-License-Identifier: Apache-2.0
 *
 * Copyright 2023 Damian Peckett <damian@pecke.tt>.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	zaplogfmt "github.com/jsternberg/zap-logfmt"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sys/unix"
)

func main() {
	config := zap.NewProductionEncoderConfig()
	logger := zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(config),
		os.Stdout,
		zap.NewAtomicLevelAt(zapcore.InfoLevel),
	))

	app := &cli.App{
		Name:  "devsyncer",
		Usage: "Synchronize device nodes between two directories with optional glob-based filters.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "source",
				Aliases:  []string{"s"},
				Usage:    "Source directory path.",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "destination",
				Aliases:  []string{"d"},
				Usage:    "Destination directory path.",
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:  "filter",
				Usage: "Glob patterns to filter device nodes to be synchronized. Can be used multiple times.",
			},
		},
		Action: func(c *cli.Context) error {
			source := c.String("source")
			destination := c.String("destination")
			filters := c.StringSlice("filter")

			return runSync(logger, source, destination, filters)
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal("Failed to run application", zap.Error(err))
	}
}

func runSync(logger *zap.Logger, source, destination string, filters []string) error {
	if err := syncDeviceNodes(logger, source, destination, filters); err != nil {
		return fmt.Errorf("error during initial sync: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating watcher: %w", err)
	}
	defer watcher.Close()

	if err = watcher.Add(source); err != nil {
		return fmt.Errorf("error adding source directory to watcher: %w", err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				srcPath := event.Name

				relPath, _ := filepath.Rel(source, srcPath)

				if !shouldSync(relPath, filters) {
					continue
				}

				destPath := filepath.Join(destination, relPath)

				fi, err := os.Stat(srcPath)
				if err != nil {
					logger.Error("Error reading file info", zap.String("path", srcPath), zap.Error(err))

					continue
				}

				if fi.IsDir() {
					if err := os.MkdirAll(destPath, fi.Mode()); err != nil {
						logger.Error("Error creating directory", zap.String("path", destPath), zap.Error(err))

						continue
					}
				} else if fi.Mode()&os.ModeDevice != 0 {
					logger.Info("Syncing device node", zap.String("device", relPath))

					stat, _ := fi.Sys().(*syscall.Stat_t)
					if err := createDeviceNode(destPath, stat.Mode, stat.Rdev); err != nil {
						logger.Error("Error creating device node", zap.String("path", destPath), zap.Error(err))

						continue
					}
				}
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				srcPath := event.Name
				relPath, _ := filepath.Rel(source, srcPath)

				if !shouldSync(relPath, filters) {
					continue
				}

				destPath := filepath.Join(destination, relPath)

				if _, err := os.Stat(destPath); err == nil {
					logger.Info("Deleting device node", zap.String("node", relPath))

					if err := os.Remove(destPath); err != nil {
						logger.Error("Error deleting device node", zap.String("path", destPath), zap.Error(err))

						continue
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}

			return fmt.Errorf("error watching source directory: %w", err)
		}
	}
}

func syncDeviceNodes(logger *zap.Logger, source, destination string, filters []string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		if !shouldSync(relPath, filters) {
			return nil
		}

		destPath := filepath.Join(destination, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		if info.Mode()&os.ModeDevice != 0 {
			logger.Info("Syncing device node", zap.String("device", relPath))

			stat, _ := info.Sys().(*syscall.Stat_t)
			return createDeviceNode(destPath, stat.Mode, stat.Rdev)
		}

		return nil
	})
}

func shouldSync(relPath string, filters []string) bool {
	for _, filter := range filters {
		if match, _ := filepath.Match(filter, filepath.Base(relPath)); match {
			return true
		}
	}

	return len(filters) == 0
}

func createDeviceNode(path string, mode uint32, dev uint64) error {
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove existing device node: %w", err)
		}
	}

	return unix.Mknod(path, mode, int(dev))
}
