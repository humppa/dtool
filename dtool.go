// dtool, Copyright (c) 2017 Tuomas Starck

package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/guilhermehn/dhash"
)

type result struct {
	err  error
	hash string
	path string
}

type fileinfo struct {
	size int64
	res  string
	md5  string
	name string
}

const (
	dhashMagicValue = 8
	cacheFilename   = ".hashcache"
	usageParallel   = "Max number of parallel jobs"
	usageVerbose    = "Print hash for each new file"
	usageVisual     = "Show duplicate images with an image viewer"
)

var parallel int
var verbose bool
var visual bool

func init() {
	runtime.GOMAXPROCS(parallel)
	log.SetFlags(log.Lshortfile)
	flag.IntVar(&parallel, "j", 1, usageParallel)
	flag.BoolVar(&verbose, "v", false, usageVerbose)
	flag.BoolVar(&visual, "V", false, usageVisual)
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

func getFileSize(filename string) (ret int64) {
	if tmp, err := os.Stat(filename); err != nil {
		log.Fatal(err)
	} else {
		ret = tmp.Size()
	}

	return
}

func getMD5(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	h := md5.New()

	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func getImageResolution(filename string) (res string) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	if i, _, err := image.DecodeConfig(f); err != nil {
		log.Fatal(err)
	} else {
		res = fmt.Sprintf("%dx%d", i.Width, i.Height)
	}

	return
}

func getFileInfo(filename string) fileinfo {
	return fileinfo{
		size: getFileSize(filename),
		md5:  getMD5(filename),
		res:  getImageResolution(filename),
		name: filename,
	}
}

func printInfo(i fileinfo) {
	fmt.Printf("%8.2f kib %10s  MD5(%s)  %s\n", float64(i.size)/1024, i.res, i.md5, i.name)
}

func displayImages(f1 string, f2 string) {
	viewer := os.Getenv("IMAGEVIEWER")
	err := exec.Command(viewer, f1, f2).Run()
	if err != nil {
		log.Fatal(err)
	}
}

func imageInfo(file1 string, file2 string) {
	f1 := getFileInfo(file1)
	f2 := getFileInfo(file2)

	if f1.md5 == f2.md5 {
		f1.md5 = "eq"
		f2.md5 = "eq"
	} else {
		f1.md5 = f1.md5[:8]
		f2.md5 = f2.md5[:8]
	}

	printInfo(f1)
	printInfo(f2)
}

func notifyCollision(hash string, file1 string, file2 string) {
	if visual {
		imageInfo(file1, file2)
		displayImages(file1, file2)
		fmt.Println()
	} else {
		fmt.Println(hash, file1, file2)
	}
}

func checkDuplicates(hashmap map[string]string) {
	lookup := make(map[string]string)

	for f2, hash := range hashmap {
		if _, err := os.Stat(f2); os.IsNotExist(err) {
			delete(hashmap, f2)
		} else if f1, ok := lookup[hash]; ok {
			notifyCollision(hash, f1, f2)
			lookup[hash] = f2
		} else {
			lookup[hash] = f2
		}
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

	writeCache(hashmap)

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
