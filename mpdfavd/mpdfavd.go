/* Copyright (C) 2013 Vincent Petithory <vincent.petithory@gmail.com>
 *
 * This file is part of mpdfav.
 *
 * mpdfav is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mpdfav is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with mpdfav.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"flag"
	"fmt"
	. "github.com/vincent-petithory/mpdclient"
	. "github.com/vincent-petithory/mpdfav"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	defaultConfigFile string = os.ExpandEnv("$HOME/.mpdfav.json")
	configFile        string
	conf              *Config
)

func startMpdService(mpdc *MPDClient, service func(*MPDClient, []chan SongSticker, chan bool), songStickerChangeHandlers []songStickerChangeHandler, wg *sync.WaitGroup, gate *Gate) {
	wg.Add(len(songStickerChangeHandlers))
	channels := make([]chan SongSticker, len(songStickerChangeHandlers))
	for i, songStickerChangeHandler := range songStickerChangeHandlers {
		ch := make(chan SongSticker)
		channels[i] = ch
		handler := songStickerChangeHandler
		go func() {
			defer wg.Done()
			ListenSongStickerChange(ch, handler)
		}()
	}
	wg.Add(1)
	go func() {
		defer func() {
			for i, _ := range channels {
				close(channels[i])
			}
			// Notify all services to shutdown
			gate.Open()
			wg.Done()
		}()
		service(mpdc, channels, gate.Waiter())
	}()
}

func startMpdServices() {
	var wg sync.WaitGroup

	var mpdc *MPDClient
	var err error
	if conf.MPDPassword != "" {
		mpdc, err = ConnectAuth(conf.MPDHost, conf.MPDPort, conf.MPDPassword)
	} else {
		mpdc, err = Connect(conf.MPDHost, conf.MPDPort)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer mpdc.Close()

	gate := NewGate()

	if conf.PlaycountsEnabled {
		startMpdService(mpdc, RecordPlayCounts, []songStickerChangeHandler{generateMostPlayedSongs(mpdc, conf.MostPlayedPlaylistName, conf.MostPlayedPlaylistLimit)}, &wg, &gate)
		log.Println("Started Playcounts service...")
	}
	if conf.RatingsEnabled {
		startMpdService(mpdc, ListenRatings, []songStickerChangeHandler{generateBestRatedSongs(mpdc, conf.BestRatedPlaylistName, conf.BestRatedPlaylistLimit)}, &wg, &gate)
		log.Println("Started Ratings service...")
	}
	wg.Wait()
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	flag.StringVar(&configFile, "config-file", defaultConfigFile, fmt.Sprintf("Use this config file instead of %s", defaultConfigFile))
}

func main() {
	conf = DefaultConfig()
	flag.Parse()
	f, err := os.Open(configFile)
	if err != nil {
		if configFile == defaultConfigFile {
			log.Println("Default config file not found")
			log.Printf("Writing default config in %s\n", defaultConfigFile)
			df, err := os.OpenFile(defaultConfigFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				log.Fatal(err)
			}
			conf.WriteTo(df)
			df.Close()
			log.Printf("Please edit and restart %s\n", filepath.Base(os.Args[0]))
			os.Exit(0)
		} else {
			log.Fatal(err)
		}
	} else {
		n, err := conf.ReadFrom(f)
		if err != nil {
			log.Fatal(err)
		}
		if n == 0 {
			log.Fatalf("No data could be read from %s\n", configFile)
		}
		f.Close()
	}

	for {
		startMpdServices()
		log.Println("Restarting...")
	}
}
