//go:build mage

package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/magefile/mage/sh"
)

func download(url, dst string) error {
	fmt.Printf("Downloading %s to %s\n", url, dst)
	// Create the file
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func unzip(name string) error {
	fmt.Printf("Unzipping %s\n", name)
	reader, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		in, err := file.Open()
		if err != nil {
			return err
		}
		defer in.Close()

		dir := path.Dir(name)
		os.MkdirAll(dir, 0777)

		out, err := os.Create(path.Join(dir, file.Name))
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		if err != nil {
			return err
		}
	}

	return nil
}

const artifactURL = "https://github.com/jcchavezs/coraza-http-wasm/releases/download/{version}/coraza-http-wasm-{version}.zip"

func downloadHTTPWasmArtifact(version, dir string) error {
	url := strings.Replace(artifactURL, "{version}", version, 2)
	return errors.Join(
		download(url, path.Join(dir, "coraza-http-wasm.zip")),
		unzip(path.Join(dir, "coraza-http-wasm.zip")),
		os.Remove(path.Join(dir, "coraza-http-wasm.zip")),
	)
}

func getHttpWasmVersion() (string, error) {
	version := os.Getenv("VERSION")
	if version == "" {
		var err error
		version, err = sh.Output("gh", "api", "repos/jcchavezs/coraza-http-wasm/releases", "-q", ".[0].tag_name")
		if err != nil {
			return "", err
		}
	}
	return version, nil
}

func DownloadArtifact() error {
	var err error
	if err = os.Mkdir("build", 0755); err != nil && !os.IsExist(err) {
		return err
	}

	version, err := getHttpWasmVersion()
	if err != nil {
		return err
	}

	return downloadHTTPWasmArtifact(version, "./build")

}

func copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func E2E() error {
	var err error

	if err = copy(".traefik.yml", "build/.traefik.yml"); err != nil {
		return err
	}

	if err = sh.RunV("docker-compose", "--file", "docker-compose.yml", "--env-file", "e2e/.env", "up", "-d", "traefik"); err != nil {
		return err
	}
	defer func() {
		_ = sh.RunV("docker-compose", "--file", "docker-compose.yml", "down", "-v")
	}()

	proxyHost := os.Getenv("TRAEFIK_HOST")
	if proxyHost == "" {
		proxyHost = "localhost:8080"
	}
	httpbinHost := os.Getenv("HTTPBIN_HOST")
	if httpbinHost == "" {
		httpbinHost = "localhost:8000"
	}

	if err = sh.RunV("go", "run", "github.com/corazawaf/coraza/v3/http/e2e/cmd/httpe2e@main", "--proxy-hostport",
		"http://"+proxyHost, "--httpbin-hostport", "http://"+httpbinHost); err != nil {
		sh.RunV("docker-compose", "-f", "docker-compose.yml", "logs", "traefik")
	}

	return err
}
