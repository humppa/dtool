// dtool, Copyright (c) 2017 Tuomas Starck

/* TODO
 * use https://golang.org/pkg/os/#Chdir
 * build a map https://blog.golang.org/go-maps-in-action for quick lookup
 * serialize/deserialize the map: https://golang.org/pkg/encoding/json/
 * figure out some way to compare and find matches
 */

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/guilhermehn/dhash"
)

type result struct {
	err  error
	hash string
	path string
}

const (
	dhashMagicValue    = 8
	defaultParallelism = 1
	usageParallel      = "Max number of parallel jobs"
)

var parallel int

func init() {
	flag.IntVar(&parallel, "j", defaultParallelism, usageParallel)
}

func getFileOrDir(unknown string) (ret []string) {
	stat, err := os.Stat(unknown)

	if err != nil {
		// Catch if unknown does not exists and any other error
		fmt.Fprintf(os.Stderr, "err: '%s': %s\n", unknown, err.Error())
		os.Exit(1)
	} else if stat.IsDir() {
		dir := filepath.Clean(unknown)

		files, err := ioutil.ReadDir(dir)

		if err != nil {
			fmt.Fprintf(os.Stderr, "err: %s\n", err.Error())
			os.Exit(1)
		}

		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".jpg" {
				ret = append(ret, filepath.Clean(filepath.Join(dir, f.Name())))
			}
		}
	} else {
		ret = append(ret, filepath.Clean(unknown))
	}

	return
}

func process(files []string) {
	var n int = len(files)

	queue := make(chan result, parallel)

	for _, path := range files {
		go func(path string) {
			hash, err := dhash.Dhash(path, dhashMagicValue)
			queue <- result{err, hash, path}
		}(path)
	}

	for i := 0; i < n; i++ {
		res := <-queue

		if res.err == nil {
			fmt.Println(res.hash, res.path)
		} else {
			fmt.Fprintf(os.Stderr, "err: '%s': %s\n", res.path, res.err.Error())
		}
	}
}

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(parallel)

	// Rest of the arguments after doubledash
	// i.e. after 'flag' has stopped parsing
	argv := os.Args[len(os.Args)-flag.NArg():]

	for _, x := range argv {
		process(getFileOrDir(x))
	}
}
