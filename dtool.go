// dtool, Copyright (c) 2017 Tuomas Starck

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	dhashMagicValue = 8
	cacheFilename   = ".hashcache"
	usageParallel   = "Max number of parallel jobs"
	usageVerbose    = "Print hash for each new file"
)

var parallel int
var verbose bool

func init() {
	runtime.GOMAXPROCS(parallel)
	log.SetFlags(log.Lshortfile)
	flag.IntVar(&parallel, "j", 1, usageParallel)
	flag.BoolVar(&verbose, "v", false, usageVerbose)
}

func chdir(unknown string) {
	stat, err := os.Stat(unknown)

	if err != nil {
		// Catch if 'unknown' does not exists or any other IO error
		log.Fatal(err.Error())
	} else if stat.IsDir() {
		dir := filepath.Clean(unknown)
		os.Chdir(dir)
	} else {
		log.Fatalf("not a directory: %s", unknown)
	}

	return
}

func readCache() (ret map[string]string) {
	ret = make(map[string]string)

	cache, err := ioutil.ReadFile(cacheFilename)

	if err != nil {
		// Fail silently if cache does not exist
		return
	}

	err = json.Unmarshal(cache, &ret)

	if err != nil {
		log.Fatal("unmarshal: cache might be corrupted")
	}

	return
}

func getDirContents(hashmap map[string]string) (ret []string) {
	files, err := ioutil.ReadDir(".")

	if err != nil {
		log.Fatal(err.Error())
	}

	filetypes := map[string]bool{
		".jpg": true,
		".png": true,
	}

	for _, f := range files {
		if !f.IsDir() {
			if _, ok := filetypes[filepath.Ext(f.Name())]; ok {
				if _, ok := hashmap[f.Name()]; !ok {
					ret = append(ret, f.Name())
				}
			}
		}
	}

	return
}

func checkDuplicates(hashmap map[string]string) {
	lookup := make(map[string]string)

	for path, hash := range hashmap {
		if fn, ok := lookup[hash]; ok {
			fmt.Println(hash, fn, "<>", path)
		}
		lookup[hash] = path
	}
}

func writeCache(hashmap map[string]string) {
	bytes, err := json.Marshal(hashmap)

	if err != nil {
		log.Fatal(err.Error())
	}

	err = ioutil.WriteFile(cacheFilename, bytes, 0644)

	if err != nil {
		log.Fatal(err.Error())
	}
}

func process() {
	hashmap := readCache()
	files := getDirContents(hashmap)
	semaphore := make(chan struct{}, parallel)
	msg := make(chan result)
	n := len(files)

	for _, path := range files {
		go func(path string) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			hash, err := dhash.Dhash(path, dhashMagicValue)
			msg <- result{err, hash, path}
		}(path)
	}

	for i := 0; i < n; i++ {
		if res := <-msg; res.err == nil {
			hashmap[res.path] = res.hash
			if verbose {
				fmt.Println(res.hash, res.path)
			}
		} else {
			fmt.Fprintf(os.Stderr, "err: '%s': %s\n", res.path, res.err.Error())
		}
	}

	checkDuplicates(hashmap)

	writeCache(hashmap)
}

func main() {
	flag.Parse()

	// Rest of the arguments after doubledash
	// i.e. after 'flag' has stopped parsing
	argv := os.Args[len(os.Args)-flag.NArg():]

	for _, path := range argv {
		chdir(path)
		process()
	}
}
