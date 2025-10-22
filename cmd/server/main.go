package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Albert12932/weather-service/internal/client/http/geocoding"
	"github.com/Albert12932/weather-service/internal/client/http/openMeteo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-co-op/gocron/v2"
)

const httpPort = ":3000"
const city = "Moscow"

type Reading struct {
	Name        string    `json:"name"`
	Timestamp   time.Time `json:"timestamp"`
	Temperature float64   `json:"temperature"`
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	ctx := context.Background()

	err := godotenv.Load()
	if err != nil {
		panic(err)
	}

	pool, err := pgxpool.New(ctx,
		fmt.Sprintf("postgres://%s:%s@localhost:54321/%s",
			os.Getenv("POSTGRES_USER"),
			os.Getenv("POSTGRES_PASSWORD"),
			os.Getenv("POSTGRES_DB")),
	)

	if err != nil {
		panic(err)
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		panic(err)
	}

	r.Get("/{city}", func(w http.ResponseWriter, r *http.Request) {
		cityName := chi.URLParam(r, "city")

		fmt.Printf("Requested city: %s\n", cityName)

		var reading Reading

		err = pool.QueryRow(ctx,
			"SELECT name, timestamp, temperature from readings where name = $1 order by timestamp desc limit 1",
			cityName,
		).Scan(&reading.Name, &reading.Timestamp, &reading.Temperature)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("No data for this city"))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal error"))
			return

		}

		var raw []byte
		raw, err = json.Marshal(reading)
		if err != nil {
			log.Println(err)
		}

		_, err = w.Write(raw)
		if err != nil {
			log.Println(err)
		}

	})

	s, err := gocron.NewScheduler()
	if err != nil {
		panic(err)
	}

	jobs, err := initJobs(s, ctx, pool)
	if err != nil {
		panic(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		fmt.Println("starting server on port", httpPort)
		err := http.ListenAndServe(httpPort, r)
		if err != nil {
			panic(err)
		}
	}()

	go func() {
		defer wg.Done()
		fmt.Printf("starting job: %v\n", jobs[0].ID())
		s.Start()
	}()

	wg.Wait()
}

func initJobs(scheduler gocron.Scheduler, ctx context.Context, pool *pgxpool.Pool) ([]gocron.Job, error) {

	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	geocodingClient := geocoding.NewClient(httpClient)
	openMetClient := openMeteo.NewClient(httpClient)

	j, err := scheduler.NewJob(
		gocron.DurationJob(
			5*time.Minute,
		),
		gocron.NewTask(
			func() {
				geoRes, err := geocodingClient.GetCoords(city)
				if err != nil {
					log.Println(err)
					return
				}

				openMetRes, err := openMetClient.GetTemperature(geoRes.Latitude, geoRes.Longitude)
				if err != nil {
					log.Println(err)
					return
				}

				loc, _ := time.LoadLocation("Europe/Moscow")

				timestamp, err := time.Parse("2006-01-02T15:04", openMetRes.Current.Time)
				if err != nil {
					log.Println(err)
					return
				}
				timestamp = timestamp.In(loc)

				pool.Exec(ctx, "INSERT INTO readings (name, timestamp, temperature) VALUES ($1, $2, $3)",
					city,
					timestamp,
					openMetRes.Current.Temperature2m)

				fmt.Printf("%v updated data for city: %s\n", timestamp, city)
			},
		),
	)
	if err != nil {
		return nil, err
	}

	return []gocron.Job{j}, nil
}
