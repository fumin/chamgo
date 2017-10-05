// This script allows us to play with the crazystone Champion Go iOS app on arbitrary board configurations.
// It achieves this by replacing the board of the latest engine server game with the latest on-device (most probably human-human) game.
// Note that the alternative of creating a new file in the game-online directory does not work, since the app uses the Game Center instead of traversing the directory to get the list of saved games.
// This is done through the backup feature of iOS, and you might need the iMazing app to extract and restore iOS backups.

package main

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

var inAvx = flag.String("a", "", "input Champion Go archive")
var player = flag.String("p", "b", "the color of the human player")

func getSavedDate(body []byte) (int32, error) {
	b := body[60:64]
	buf := bytes.NewReader(b)
	var t int32
	if err := binary.Read(buf, binary.LittleEndian, &t); err != nil {
		return 0, fmt.Errorf("parse %v error: %v", b, err)
	}
	return t, nil
}

func readAvx(f string, online bool) (string, []byte, error) {
	r, err := zip.OpenReader(f)
	if err != nil {
		return "", nil, err
	}
	defer r.Close()

	prefix := "Container/Documents/game/"
	if online {
		prefix = "Container/Documents/game-online/"
	}

	var latest string
	var latestBody []byte
	var latestDate int32 = -1
	for _, f := range r.File {
		if !filepath.HasPrefix(f.Name, prefix) {
			continue
		}
		if f.Mode().IsDir() {
			continue
		}
		body, err := func() ([]byte, error) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			body, err := ioutil.ReadAll(rc)
			if err != nil {
				return nil, err
			}
			return body, nil
		}()
		if err != nil {
			return "", nil, err
		}
		savedDate, err := getSavedDate(body)
		if err != nil {
			return "", nil, err
		}
		if savedDate > latestDate {
			latest = f.Name
			latestBody = body
			latestDate = savedDate
		}
	}

	return latest, latestBody, nil
}

func flipBoard180(body []byte) {
	// board size
	bs := body[8]

	for i := 76; i < len(body); i += 20 {
		body[i+4] = bs - body[i+4] + 1
		body[i+8] = bs - body[i+8] + 1
	}
}

func flipToComputer(body []byte) {
	// The 5th byte determines that it is a computer game.
	//body[4] = 0 // computer vs human
	body[4] = 1 // human vs human

	// The 12th byte determines whether the human player is black or white.
	// If it is 0 then human plays black.
	if *player == "w" {
		body[12] = 1
		flipBoard180(body)
	} else {
		body[12] = 0
	}

	// Level 10 computer
	body[16] = 0x0a

	// Update the started and save dates to make it easier to find
	buf := bytes.NewBuffer(body[56:56])
	now := int32(time.Now().Unix())
	binary.Write(buf, binary.LittleEndian, now) // started date
	binary.Write(buf, binary.LittleEndian, now) // saved date
}

func writeAvx(w io.Writer, avxName string, latestBody []byte, firstOnline string) error {
	zw := zip.NewWriter(w)
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})

	r, err := zip.OpenReader(avxName)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		err := func() error {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			of, err := zw.Create(f.Name)
			if err != nil {
				return err
			}

			if f.Name == firstOnline {
				_, err = of.Write(latestBody)
				if err != nil {
					return err
				}
			} else {
				_, err = io.Copy(of, rc)
				if err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return nil
}

func main() {
	flag.Parse()
	_, latestBody, err := readAvx(*inAvx, false)
	if err != nil {
		log.Fatal(err)
	}
	firstOnline, _, err := readAvx(*inAvx, true)
	if err != nil {
		log.Fatal(err)
	}

	flipToComputer(latestBody)

	if err := writeAvx(os.Stdout, *inAvx, latestBody, firstOnline); err != nil {
		log.Fatal(err)
	}
}
