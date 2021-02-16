# Measuring the performance of pg_trgm

This repository contains **extensive, verbose, detailed** information about the behavior of pg_trgm at large scales, particularly for regex search.

Before viewing this repository, you will likely prefer to read the blog post format first:

["Postgres regex search over 10,000 GitHub repositories"](https://devlog.hexops.com/2021/postgres-regex-search-over-10000-github-repositories)

This repository shares how we performed our empirical measurements, for reproducibility by others.

## Overview

- `cmd/corpusindex` small Go program which bulk inserts the corpus into Postgres
- `cmd/githubscrape` small Go program that fetches the top 1,000 repositories for any language.
- `cmd/visualize-docker-json-stats` cleans up `docker_stats_logs/` output for visualization using [the jp tool](https://github.com/sgreben/jp).
- `docker_logs/` logs from the Docker container during execution.
- `docker_stats_logs/` logs from `docker stats` during indexing/querying the corpus, showing CPU/memory usage over time.
- `top_repos/` contains URLs to the top 1,000 repositories for a given language. In total, 20,578 repositories.
- `query_logs/` the Postgres SQL queries we ultimately ran.
- `capture-docker-stats.sh` captures `docker stats` output every 1s with timing info.
- `clone-corpus.sh` clones all 20,578 repositories (concurrently.)
- `extract-base-postgres-config.sh` extracts the base Postgres config from the Docker image.
- `index-corpus.sh` used to invoke the `corpusindex` tool for every repository, once cloned.
- `query-corpus.sh` runs detailed search queries over the corpus (invokes the other `query-corpus*` scripts.)
- `run-postgres.sh` runs the Postgres server Docker image.

## Cloning the corpus

First run `./clone_corpus.sh` to download the corpus into `../corpus` (it uses the parent directory, because VS Code and most tooling will barf if there is a directory that many files existing in a project.)

WARNING, this will:

* Clone all 20,578 repositories concurrently, using most of your available CPU/memory/network resources.
* Take 12-16 hours with a fast ~100 Mbps connection to GitHub's servers.
* Consumes ~265G of disk space.
* Requires you have `gfind` installed (`brew install gfind`), otherwise replace `gfind` with `find` in the script.

To try and save you disk space, the script will already trim the data down a lot, reducing the corpus size by about 66%:

* Clones repos only with `--depth 1`
* Deletes the entire `.git` directory after cloning repos, so only file contents are left. This reduces the corpus size by about 30% (412G -> 290G, for 12,148 repos) 
* Deleting files greater than 1MB. GitHub only indexes files smaller than 384KB, for example - and this 1MB limit reduces the corpus size by _another_ whopping 51% (290G -> 142G, for 12,148 repos) - wow.

You can use this command at any time to figure out how many repos have been cloned:

```sh
echo "progress: $(find ../corpus/ -maxdepth 4 -mindepth 4 | wc -l) repos cloned"
```

## Setting up Docker

If you plan on using Docker and are on Mac OS, you are using a VM and this has performance implications. Be sure to:

1. Max out the CPUs, Memory, and disk space available to Docker.
2. Disable "Use gRPC FUSE for file sharing" in Experimental Features.

## Initializing Postgres

Launch Postgres via `./run-postgres.sh`, and then get a `psql` prompt:

```sh
docker exec -it postgres psql -U postgres
```

Create the DB schema:

```sql
BEGIN;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE IF NOT EXISTS files (
    id bigserial PRIMARY KEY,
    contents text NOT NULL,
    filepath text NOT NULL
);
COMMIT;
```

## Indexing the corpus

Index a single repository:

```sh
DATABASE=postgres://postgres@127.0.0.1:5432/postgres go run ./cmd/corpusindex/main.go ../corpus/c/github.com\\/linux-noah\\/noah/
```

Index all repositories:

```sh
go install ./cmd/corpusindex; DATABASE=postgres://postgres@127.0.0.1:5432/postgres ./index-corpus.sh
```

## Querying the corpus

```
postgres=# SELECT filepath FROM files WHERE contents ~ 'use strict';
                                   filepath                                    
-------------------------------------------------------------------------------
 ../corpus/c/github.com\/linux-noah\/noah/.git/hooks/fsmonitor-watchman.sample
(1 row)
```

This will take around ~8 hours on a 2020 Macbook Pro i9 (8 physical cores, 16 virtual) w/ 16G memory.

On-disk size of the entire DB at this point will be 101G.

## Create the Trigram index

```sql
CREATE INDEX IF NOT EXISTS files_contents_trgm_idx ON files USING GIN (contents gin_trgm_ops);
```

### Configuration attempt 1 (indexing failure, OOM)

With this configuration, the above `CREATE INDEX` command will take `11h34m` and ultimately OOM and fail:

```
listen_addresses = '*'
max_connections = 100
shared_buffers = 4GB
effective_cache_size = 12GB
maintenance_work_mem = 16GB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100
random_page_cost = 1.1
effective_io_concurrency = 200
work_mem = 5242kB
min_wal_size = 50GB
max_wal_size = 4GB
max_worker_processes = 8
max_parallel_workers_per_gather = 8
max_parallel_workers = 8
max_parallel_maintenance_workers = 8
```

```
postgres=# CREATE INDEX IF NOT EXISTS files_contents_trgm_idx ON files USING GIN (contents gin_trgm_ops);

server closed the connection unexpectedly
	This probably means the server terminated abnormally
	before or while processing the request.
The connection to the server was lost. Attempting reset: Failed.
```

Postgres logs indicate:

```
2021-01-30 05:04:11.045 GMT [276] LOG:  stats_timestamp 2021-01-30 05:04:11.773621+00 is later than collector's time 2021-01-30 05:04:11.036405+00 for database 0
2021-01-30 08:00:56.721 GMT [276] LOG:  stats_timestamp 2021-01-30 08:00:56.707853+00 is later than collector's time 2021-01-30 08:00:56.702848+00 for database 0
2021-01-30 08:24:57.919 GMT [276] LOG:  stats_timestamp 2021-01-30 08:24:57.922315+00 is later than collector's time 2021-01-30 08:24:57.917066+00 for database 13442
2021-01-30 09:05:13.815 GMT [1] LOG:  server process (PID 290) was terminated by signal 9: Killed
2021-01-30 09:05:13.815 GMT [1] DETAIL:  Failed process was running: CREATE INDEX IF NOT EXISTS files_contents_trgm_idx ON files USING GIN (contents gin_trgm_ops);
2021-01-30 09:05:13.818 GMT [1] LOG:  terminating any other active server processes
2021-01-30 09:05:13.823 GMT [275] WARNING:  terminating connection because of crash of another server process
2021-01-30 09:05:13.823 GMT [275] DETAIL:  The postmaster has commanded this server process to roll back the current transaction and exit, because another server process exited abnormally and possibly corrupted shared memory.
2021-01-30 09:05:13.823 GMT [275] HINT:  In a moment you should be able to reconnect to the database and repeat your command.
2021-01-30 09:05:13.854 GMT [980] FATAL:  the database system is in recovery mode
2021-01-30 09:05:14.020 GMT [1] LOG:  all server processes terminated; reinitializing
2021-01-30 09:08:44.448 GMT [981] LOG:  database system was interrupted; last known up at 2021-01-29 22:18:31 GMT
2021-01-30 09:08:50.772 GMT [981] LOG:  database system was not properly shut down; automatic recovery in progress
2021-01-30 09:08:50.876 GMT [981] LOG:  redo starts at 19/82EF3D98
2021-01-30 09:08:50.877 GMT [981] LOG:  invalid record length at 19/82EF3EE0: wanted 24, got 0
2021-01-30 09:08:50.877 GMT [981] LOG:  redo done at 19/82EF3EA8
2021-01-30 09:08:51.158 GMT [1] LOG:  database system is ready to accept connections
```

No postgres _container_ restart will be observed because (interestingly) Postgres can handle the OOM without restarting the container and start itself again. One of the benefits of handling C allocation failures, I presume, but didn't investigate:

```
$ docker ps
CONTAINER ID        IMAGE                  COMMAND                  CREATED             STATUS              PORTS                      NAMES
eb087868cb00        postgres:13.1-alpine   "docker-entrypoint.s…"   43 hours ago        Up 43 hours         127.0.0.1:5432->5432/tcp   postgres
```

See `docker_stats_logs/configuration-failure-1.log` for a JSON log stream of container `docker stats` captured during the `CREATE INDEX`.

There is evidence that indexing with that configuration -- for whatever reason -- for the vast majority of indexing time uses just 1-2 CPU cores, and peak ~11 GiB of memory according to `docker stats`.

Memory usage in MiB as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-failure-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=32000 | jq | jp -y '..MemUsageMiB'
```

<img width="981" alt="image" src="https://user-images.githubusercontent.com/3173176/107313722-56bbac80-6a50-11eb-94c7-8e13ea095053.png">

CPU usage percentage (150% indicates "one and a half virtual CPU cores") as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-failure-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=32000 | jq | jp -y '..CPUPerc'
```

<img width="982" alt="image" src="https://user-images.githubusercontent.com/3173176/107313915-cc277d00-6a50-11eb-9282-62159a127966.png">

Less reliable charts from a Mac app (seems to have periodic data loss issues):

Memory usage (purple == compressed, red==active, blue==wired):

<img width="354" alt="image" src="https://user-images.githubusercontent.com/3173176/106368429-9a067480-6306-11eb-82f6-769733a425ee.png">

Memory pressure:

<img width="356" alt="image" src="https://user-images.githubusercontent.com/3173176/106368408-80fdc380-6306-11eb-8169-865605b7815d.png">

Memory swap:

<img width="355" alt="image" src="https://user-images.githubusercontent.com/3173176/106368339-13519780-6306-11eb-8d14-9b0deda6ed78.png">

Disk activity:

<img width="360" alt="image" src="https://user-images.githubusercontent.com/3173176/106368350-2f553900-6306-11eb-81f5-7a017f0b9d50.png">

CPU activity:

<img width="352" alt="image" src="https://user-images.githubusercontent.com/3173176/106368381-57449c80-6306-11eb-80c0-774f23832d71.png">

CPU load avg:

<img width="346" alt="image" src="https://user-images.githubusercontent.com/3173176/106368392-6d525d00-6306-11eb-8538-d3e398486e22.png">


### Configuration attempt 2 (24h+ of indexing, then out of disk space)

For this attempt, we use a configuration provided by pgtune for a data warehouse with 10G memory to reduce the chance of OOMs:

```
# DB Version: 13
# OS Type: linux
# DB Type: dw
# Total Memory (RAM): 10 GB
# CPUs num: 8
# Connections num: 20
# Data Storage: ssd

max_connections = 20
shared_buffers = 2560MB
effective_cache_size = 7680MB
maintenance_work_mem = 1280MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 500
random_page_cost = 1.1
effective_io_concurrency = 200
work_mem = 16MB
min_wal_size = 4GB
max_wal_size = 16GB
max_worker_processes = 8
max_parallel_workers_per_gather = 4
max_parallel_workers = 8
max_parallel_maintenance_workers = 4
```

Indexing took `~26h54m`, compared to the `~11h34m` in the previous attempt, starting at 2:39pm and ending at ~5:31pm the next day in an out-of-space failure.

See `docker_stats_logs/configuration-failure-2.log` for a full JSON stream of `docker stats` during indexing.

See `logs/configuration-failure-2.log` for the Postgres logs during this attempt.

Of particular note is that, again, almost 100% of the time was spent with a single CPU core maxed out and the vast majority of the CPU in `Idle` state (red).

Memory usage in MiB as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-failure-2.log | go run ./cmd/visualize-docker-json-stats/main.go | jq | jp -y '..MemUsageMiB'
```

<img width="980" alt="image" src="https://user-images.githubusercontent.com/3173176/107314104-350ef500-6a51-11eb-909f-2f1b524d29b2.png">

CPU usage percentage (150% indicates "one and a half virtual CPU cores") as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-failure-2.log | go run ./cmd/visualize-docker-json-stats/main.go | jq | jp -y '..CPUPerc'
```

<img width="980" alt="image" src="https://user-images.githubusercontent.com/3173176/107314168-507a0000-6a51-11eb-8a18-ec18752f7f16.png">

Less reliable charts from a Mac app (seems to have periodic data loss issues):

<img width="597" alt="image" src="https://user-images.githubusercontent.com/3173176/106505762-fba11d00-6485-11eb-8c58-5954b1dfeb3a.png">

Memory pressure was mostly fine and remained under 75%:

<img width="597" alt="image" src="https://user-images.githubusercontent.com/3173176/106506305-c5b06880-6486-11eb-9aef-0a0f086fa623.png">

Memory usage (purple == compressed, red==active, blue==wired) shows we never hit memory limits or even high usage:

<img width="594" alt="image" src="https://user-images.githubusercontent.com/3173176/106506397-e5479100-6486-11eb-93eb-a4488efb15b9.png">

The `docker stats` stream (`docker_stats_logs/configuration-failure-2.log`) shows memory usage throughout the 24h+ period never going above ~1.4G.

Despite this, system swap was used somewhat heavily:

<img width="600" alt="image" src="https://user-images.githubusercontent.com/3173176/106507665-9ac71400-6488-11eb-839f-13e0369865d2.png">

Disk usage during indexing tells us that the average was about ~250 MB/s for reads (blue) and writes (red):

<img width="599" alt="image" src="https://user-images.githubusercontent.com/3173176/106507903-ec6f9e80-6488-11eb-88a8-78e5b7aacfd6.png">

It should be noted that in-software disk speed tests (i.e. including disk encryption Mac OS is performing) show regular read and write speeds of ~860 MB/s with <5% effect on CPU usage:

<img width="591" alt="image" src="https://user-images.githubusercontent.com/3173176/106508609-d8786c80-6489-11eb-92c7-b69db1ee1daa.png">

It should also be noted that postgres disk usage during this test, although eventually running out, rose from `101G` to `124G`:

```
$ du -sh .postgres/
124G	.postgres/
```

### Configuration attempt 3: reduced dataset

For this attempt, and to reduce the turnaround time on experiments, we use the same postgres configuration as in attempt 2 but we use a reduced dataset. Before we had 19,441,820 files totalling ~178.2 GiB:

```
postgres=# select count(filepath) from files;
  count   
----------
 19441820
(1 row)

postgres=# select SUM(octet_length(contents)) from files;
     sum      
--------------
 191379114802
(1 row)
```

We drop half the files in the dataset, and :

```
postgres=# select count(filepath) from files;
  count  
---------
 9720910
(1 row)

postgres=# select SUM(octet_length(contents)) from files;
     sum     
-------------
 88123563320
(1 row)
```

Now 82 GiB of raw text are to be indexed.

And we free ~228G for use by the Postgres indexing (previously ~15G.)

Index creation this time took from 3:14pm MST to 7:44pm MST (next day), a total of 28h30m. However, for some period of this time the Macbook went into low-power (not sleep) mode for - approx 6h - making actual indexing time around ~22h.

Total Postgres data size afterwards (again, less than 82 GiB due to compression):

```
$ du -sh .postgres/
 73G	.postgres/
```

Memory usage in MiB as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-3.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=0 | jq | jp -y '..MemUsageMiB'
```

<img width="980" alt="image" src="https://user-images.githubusercontent.com/3173176/107315387-ce3f0b00-6a53-11eb-886c-410f000f73bd.png">

CPU usage percentage (150% indicates "one and a half virtual CPU cores") as reported by `docker stats` over time rendered via:

```
cat ./docker_stats_logs/configuration-3.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=0 | jq | jp -y '..CPUPerc'
```

<img width="980" alt="image" src="https://user-images.githubusercontent.com/3173176/107315239-8324f800-6a53-11eb-9a5b-fcc61d1a7b59.png">


(We did not take measurements through the Mac app for indexing this time.)

## Query performance

Restart Postgres first, such that its memory caches are emptied.

Once it starts, capture docker stats:

```sh
OUT=docker_stats_logs/query-run-n.log ./capture-docker-stats.sh
```

Set a query timeoout of 5 minutes on the database:

```sql
ALTER DATABASE postgres SET statement_timeout = '300s';
```

Then begin querying the corpus:

```
./query-corpus.sh &> query_logs/query-run-1.log
```

## Query performance

We started queries at 12:42PM MST using:

```
./query-corpus.sh &> query_logs/query-run-1.log
```

- Find the exact SQL queries we ran in `query_logs/query-run-1.log`.
- Find the `docker stats` measured during query execution in `docker_stats_logs/query-run-1.log`.

CPU usage (150% == one and a half virtual cores) during query execution as visualized by:

```sh
cat ./docker_stats_logs/query-run-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=9000 | jq | jp -y '..CPUPerc'
```

<img width="1001" alt="image" src="https://user-images.githubusercontent.com/3173176/107459155-c85c2f00-6b12-11eb-9b2a-27e0f1424ed6.png">

Memory usage in MiB during query execution as visualized by:

```sh
cat ./docker_stats_logs/query-run-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=9000 | jq | jp -y '..MemUsageMiB'
```

<img width="996" alt="image" src="https://user-images.githubusercontent.com/3173176/107459238-fa6d9100-6b12-11eb-8692-4a68e421b2a6.png">


We can see there were 1458 queries total with:

```sh
$ cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[]' | jq -c -s '.[]' | wc -l
    1458
```

And of those, we can see that 20 timed out:

```sh
$ cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select (.Timeout == true)' | jq -c -s '.[]' | wc -l
      20
```

We can also visualize that 1405 (96.4%) queries completed in under 1300ms (Y axis), but are generally planned to execute in under 47ms (X axis):

```sh
cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select(.ExecutionTimeMs < 1300) | select(.PlanningTimeMs < 50)' | jq -c -s '.[]' | wc -l
```

```sh
cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select(.ExecutionTimeMs < 1300) | select(.PlanningTimeMs < 50)' | jq -s | jp -x '..PlanningTimeMs' -y '..ExecutionTimeMs' -type scatter -canvas quarter
```

<img width="980" alt="image" src="https://user-images.githubusercontent.com/3173176/107490175-c0b67d80-6b46-11eb-9124-24910290ecc6.png">


However, there are outliers which were planned for execution in under 100ms (X axis) and actually had execution between 1.3s to ~150s (Y axis):

```sh
cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select(.ExecutionTimeMs > 1300) | select(.PlanningTimeMs < 100)' | jq -s | jp -x '..PlanningTimeMs' -y '..ExecutionTimeMs' -type scatter -canvas quarter
```

<img width="981" alt="image" src="https://user-images.githubusercontent.com/3173176/107491036-e1330780-6b47-11eb-845b-0dafaa1cff44.png">

We can see the above accounted for 29 queries via:

```
cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select(.ExecutionTimeMs > 1300) | select(.PlanningTimeMs < 100)' | jq -c -s '.[]' | wc -l
```

We can also see how many of which types of queries we ran, e.g. via:

```
$ cat ./query_logs/query-run-1.log | go run ./cmd/visualize-query-log/main.go | jq -c '.[] | {Timeout: .Timeout, Limit: .Limit, Results: .Rows, Query: .Query}' | sort | uniq -c
 100 {"Timeout":false,"Limit":10,"Results":10,"Query":"123456789"}
   2 {"Timeout":false,"Limit":10,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
   4 {"Timeout":false,"Limit":10,"Results":10,"Query":"bytes.Buffer"}
   2 {"Timeout":false,"Limit":10,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
 100 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Error"}
 100 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Print.*"}
 100 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Println"}
 200 {"Timeout":false,"Limit":10,"Results":7,"Query":"error"}
 100 {"Timeout":false,"Limit":10,"Results":7,"Query":"var"}
 100 {"Timeout":false,"Limit":100,"Results":10,"Query":"123456789"}
   2 {"Timeout":false,"Limit":100,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
   4 {"Timeout":false,"Limit":100,"Results":10,"Query":"bytes.Buffer"}
   2 {"Timeout":false,"Limit":100,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
 100 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Error"}
 100 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Print.*"}
 100 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Println"}
 200 {"Timeout":false,"Limit":100,"Results":7,"Query":"error"}
 100 {"Timeout":false,"Limit":100,"Results":7,"Query":"var"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"123456789"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
   4 {"Timeout":false,"Limit":1000,"Results":10,"Query":"bytes.Buffer"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Error"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Print.*"}
   2 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Println"}
   4 {"Timeout":false,"Limit":1000,"Results":7,"Query":"error"}
   2 {"Timeout":false,"Limit":1000,"Results":7,"Query":"var"}
   4 {"Timeout":true,"Limit":-1,"Results":10,"Query":"error"}
   2 {"Timeout":true,"Limit":-1,"Results":10,"Query":"fmt\\.Error"}
   1 {"Timeout":true,"Limit":-1,"Results":10,"Query":"fmt\\.Println"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"123456789"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"bytes.Buffer"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"fmt\\.Print.*"}
   1 {"Timeout":true,"Limit":-1,"Results":9,"Query":"fmt\\.Println"}
   2 {"Timeout":true,"Limit":-1,"Results":9,"Query":"var"}
```

### Query execution #2

We rerun with much higher number of queries executed, and with timeout raised to 1h:

```sql
ALTER DATABASE postgres SET statement_timeout = '3600s';
```

We start at 3:06AM MST , and execution ran until 7:21PM MST the next day. At this point, `query-corpus-unlimited.sh` was configured to execute each query 100 times. Due to how slow these queries were, we estimated it would take ~11 days to complete and halted testing to reduce the number of queries to just 2 executions per query. These runs are recorded in `query_logs/query-run-[2-3].log` and `docker_stats_logs/query-run-[2-3].log` respectively.

The third attempt executed only partially, in specific it executed until the queries for unlimited/unbounded `var`:

```
EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'var') as e;
```

Once the first unbounded `var` query described above tried to run, we found the machine appeared to be off unexpectedly. MacOS ran out of resources (likely CPU, but possibly memory - and definitely not disk space) causing MacOS's kernel watchdog extension to panic ultimately bricking the entire MacOS installation requiring us to reinstall it: https://twitter.com/slimsag/status/1360513988514091010

### Query execution #2 & 3 performance

#### What queries were ran?

Total number of queries ran:

```
$ cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq -c '.[] | {Timeout: .Timeout, Limit: .Limit, Results: .Rows, Query: .Query}' | wc -l
   19936
```

Exact numbers of each query type ran:

```
$ cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq -c '.[] | {Timeout: .Timeout, Limit: .Limit, Results: .Rows, Query: .Query}' | sort | uniq -c | sort
   2 {"Timeout":false,"Limit":-1,"Results":9,"Query":"fmt\\.Error"}
   2 {"Timeout":false,"Limit":-1,"Results":9,"Query":"fmt\\.Print.*"}
   2 {"Timeout":false,"Limit":-1,"Results":9,"Query":"fmt\\.Println"}
   4 {"Timeout":false,"Limit":-1,"Results":17,"Query":"error"}
   4 {"Timeout":false,"Limit":10,"Results":10,"Query":"bytes.Buffer'"}
   4 {"Timeout":false,"Limit":1000,"Results":10,"Query":"bytes.Buffer'"}
  18 {"Timeout":false,"Limit":-1,"Results":17,"Query":"error'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"123456789'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Error'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Print.*'"}
 100 {"Timeout":false,"Limit":1000,"Results":10,"Query":"fmt\\.Println'"}
 100 {"Timeout":false,"Limit":1000,"Results":7,"Query":"var'"}
 200 {"Timeout":false,"Limit":1000,"Results":7,"Query":"error'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"123456789'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Error'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Print.*'"}
1000 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Println'"}
1000 {"Timeout":false,"Limit":10,"Results":7,"Query":"var'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"123456789'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"ac8ac5d63b66b83b90ce41a2d4061635'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"bytes.Buffer'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"d97f1d3ff91543[e-f]49.8b07517548877'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Error'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Print.*'"}
1000 {"Timeout":false,"Limit":100,"Results":10,"Query":"fmt\\.Println'"}
1000 {"Timeout":false,"Limit":100,"Results":7,"Query":"var'"}
2000 {"Timeout":false,"Limit":10,"Results":7,"Query":"error'"}
2000 {"Timeout":false,"Limit":100,"Results":7,"Query":"error'"}
```

#### How many timed out?

```
$ cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select (.Timeout == true)' | jq -c -s '.[]' | wc -l
       0
```

(But we should include the last single query which bricked our MacOS installation mentioned previously..)

#### CPU/memory usage

CPU usage (150% == one and a half virtual cores) during query execution as visualized by:

```sh
cat ./docker_stats_logs/query-run-2.log ./docker_stats_logs/query-run-3.log | go run ./cmd/visualize-docker-json-stats/main.go | jq | jp -y '..CPUPerc'
```

<img width="1253" alt="image" src="https://user-images.githubusercontent.com/3173176/107847580-0f178680-6daa-11eb-8474-afe2f057cedb.png">

Memory usage in MiB during query execution as visualized by:

```sh
cat ./docker_stats_logs/query-run-2.log ./docker_stats_logs/query-run-3.log | go run ./cmd/visualize-docker-json-stats/main.go | jq | jp -y '..MemUsageMiB'
```

<img width="1251" alt="image" src="https://user-images.githubusercontent.com/3173176/107847596-30787280-6daa-11eb-979c-b77c3711b948.png">

The large spike towards the end is a result of beginning to execute `query-corpus-unlimited.sh` queries - i.e. ones without any `LIMIT`.

#### Other measurements

We can determine how many queries executed in under a time bucket using e.g.:

```
$ cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select (.ExecutionTimeMs < 25000)' | jq -c -s '.[]' | wc -l
```

Or this to get a scatter plot showing planning time for the query (X axis) vs. execution time for the query (Y axis):

```
cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq '.[]' | jq -s | jp -x '..PlanningTimeMs' -y '..ExecutionTimeMs' -type scatter -canvas quarter
```

Or this to do the same, but only include queries that executed in under 5s:

```
cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select(.ExecutionTimeMs < 5000)' | jq -s | jp -x '..PlanningTimeMs' -y '..ExecutionTimeMs' -type scatter -canvas quarter
```

We can also plot execution time (Y axis) vs. # of index rechecks (X axis):

```
cat ./query_logs/query-run-2.log ./query_logs/query-run-3.log | go run ./cmd/visualize-query-log/main.go | jq '.[]' | jq -s | jp -x '..IndexRechecks' -y '..ExecutionTimeMs' -type scatter -canvas quarter
```

<img width="1036" alt="image" src="https://user-images.githubusercontent.com/3173176/107849660-fc0cb280-6db9-11eb-9c10-cb7e74366ab7.png">


### Database startup time

Clean startups are almost instantaneous, taking less than a second. 

If the DB is not shut down correctly (i.e. previously terminated during startup), startup takes a fairly hefty 10m12s to complete before the DB will accept any connections, as it begins a recovery process (which I assume involves reading a substantial portion of the DB from disk):

```
PostgreSQL Database directory appears to contain a database; Skipping initialization

2021-02-08 21:45:48.452 GMT [1] LOG:  starting PostgreSQL 13.1 on x86_64-pc-linux-musl, compiled by gcc (Alpine 9.3.0) 9.3.0, 64-bit
2021-02-08 21:45:48.454 GMT [1] LOG:  listening on IPv4 address "0.0.0.0", port 5432
2021-02-08 21:45:48.454 GMT [1] LOG:  listening on IPv6 address "::", port 5432
2021-02-08 21:45:48.531 GMT [1] LOG:  listening on Unix socket "/var/run/postgresql/.s.PGSQL.5432"
2021-02-08 21:45:48.633 GMT [21] LOG:  database system was interrupted; last known up at 2021-02-03 06:16:10 GMT
2021-02-08 21:47:51.157 GMT [27] FATAL:  the database system is starting up
2021-02-08 21:47:56.383 GMT [33] FATAL:  the database system is starting up
2021-02-08 21:48:13.198 GMT [39] FATAL:  the database system is starting up
2021-02-08 21:48:43.088 GMT [45] FATAL:  the database system is starting up
2021-02-08 21:52:43.672 GMT [51] FATAL:  the database system is starting up
2021-02-08 21:53:32.048 GMT [58] FATAL:  the database system is starting up
2021-02-08 21:54:07.696 GMT [64] FATAL:  the database system is starting up
2021-02-08 21:55:36.446 GMT [21] LOG:  database system was not properly shut down; automatic recovery in progress
2021-02-08 21:55:36.515 GMT [21] LOG:  redo starts at 2B/EE02EE8
2021-02-08 21:55:36.518 GMT [21] LOG:  invalid record length at 2B/EE02FD0: wanted 24, got 0
2021-02-08 21:55:36.518 GMT [21] LOG:  redo done at 2B/EE02F98
2021-02-08 21:55:36.783 GMT [1] LOG:  database system is ready to accept connections
```

### Data size (total on disk)

After indexing:

```
$ du -sh .postgres/
 73G	.postgres/
```

After `DROP INDEX files_contents_trgm_idx;`:

```
$ du -sh .postgres/
 54G	.postgres/
```

### Data size reported by Postgres

After indexing:

```
postgres=# \d+
                                  List of relations
 Schema |     Name     |   Type   |  Owner   | Persistence |    Size    | Description 
--------+--------------+----------+----------+-------------+------------+-------------
 public | files        | table    | postgres | permanent   | 47 GB      | 
 public | files_id_seq | sequence | postgres | permanent   | 8192 bytes | 
(2 rows)
```

After `DROP INDEX files_contents_trgm_idx;`:

```
postgres=# \d+
                                  List of relations
 Schema |     Name     |   Type   |  Owner   | Persistence |    Size    | Description 
--------+--------------+----------+----------+-------------+------------+-------------
 public | files        | table    | postgres | permanent   | 47 GB      | 
 public | files_id_seq | sequence | postgres | permanent   | 8192 bytes | 
(2 rows)
```

## Table splitting

We did a brief experiment with table splitting to try and increase Postgres's use of multiple CPU cores, and to reduce the number of row rechecks caused by trigram false positive matches.

First we got the total number of rows:

```sql
postgres=# select count(*) from files;
  count  
---------
 9720910
(1 row)
```

Based on this, we determined that we would try 200 tables each with 50,000 rows max. We generated the queries to create the tables:

```sql
$ go run ./cmd/tablesplitgen/main.go create
CREATE TABLE files_000 AS SELECT * FROM files WHERE id > 0 AND id < 50000;
CREATE TABLE files_001 AS SELECT * FROM files WHERE id > 50000 AND id < 100000;
CREATE TABLE files_002 AS SELECT * FROM files WHERE id > 100000 AND id < 150000;
CREATE TABLE files_003 AS SELECT * FROM files WHERE id > 150000 AND id < 200000;
CREATE TABLE files_004 AS SELECT * FROM files WHERE id > 200000 AND id < 250000;
...
```

And ran those after unsetting the statement timeout:

```sql
ALTER DATABASE postgres SET statement_timeout = default;
```

Creation of these tables took around ~20-40s each - about two hours in total.

We then began recording docker stats:

```
OUT=docker_stats_logs/split-index-1.log ./capture-docker-stats.sh
```

And used a program to generate and run the SQL statement for indexing, e.g.:

```sql
CREATE INDEX IF NOT EXISTS files_000_contents_trgm_idx ON files USING GIN (contents gin_trgm_ops);
```

In parallel (8 at a time), for all 200 tables:

```sh
PARALLEL=8 go run ./cmd/tablesplitgen/main.go index &> ./index_logs/split-index-1.log
```

This failed due to an OOM about half way through at around 117 tables indexed, see `index_logs/split-index-1.log`. We then reran with 6 workers and it succeeded (one of the benefits of this approach is we did not have to reindex those 117 tables that did completed).

See `docker_stats_logs/split-index-1.log` for how long it took and CPU/memory usage, which did consume multiple CPU cores.

## Performance gains from table splitting

We took a representative sample of a slow query that previously took 27.6s:

```
query: EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'd97f1d3ff91543[e-f]49.8b07517548877' limit 1000) as e;
Timing is on.
                                                                     QUERY PLAN                                                                      
-----------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=1209.80..1209.81 rows=1 width=8) (actual time=27670.917..27670.972 rows=1 loops=1)
   ->  Limit  (cost=132.84..1197.80 rows=960 width=8) (actual time=27670.904..27670.948 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=132.84..1197.80 rows=960 width=8) (actual time=27670.894..27670.907 rows=0 loops=1)
               Recheck Cond: (contents ~ 'd97f1d3ff91543[e-f]49.8b07517548877'::text)
               Rows Removed by Index Recheck: 9838
               Heap Blocks: exact=5379
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..132.60 rows=960 width=0) (actual time=38.235..38.239 rows=9838 loops=1)
                     Index Cond: (contents ~ 'd97f1d3ff91543[e-f]49.8b07517548877'::text)
 Planning Time: 36.870 ms
 Execution Time: 27671.854 ms
(10 rows)
```

We then wrote a small program which starts N workers and in parallel executes a query against all 200 tables (ordered), until it finds the specified limit of results - finding the previously 27.6s query now takes only 7.1s to find no results:

```
$ DATABASE=postgres://postgres@127.0.0.1:5432/postgres PARALLEL=8 go run ./cmd/tablesplitgen/main.go query 'd97f1d3ff91543[e-f]49.8b07517548877' 1000 200
...
0 results in 7140ms
```

We found that higher numbers for `PARALLEL` generally improved query time, and so we raised postgres.conf `max_connections` to 128 to allow for higher number of parallel testing. Some brief testing showed that around 32 parallel connections, we no longer saw performance gains and the query executed in 5738ms.

### Take 2

We also tried another query which was previously quite slow, taking 27.3s:

```
query: EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'ac8ac5d63b66b83b90ce41a2d4061635' limit 10) as e;
Timing is on.
                                                                      QUERY PLAN                                                                      
------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=144.06..144.07 rows=1 width=8) (actual time=27379.079..27379.110 rows=1 loops=1)
   ->  Limit  (cost=132.84..143.94 rows=10 width=8) (actual time=27379.067..27379.087 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=132.84..1197.80 rows=960 width=8) (actual time=27379.038..27379.050 rows=0 loops=1)
               Recheck Cond: (contents ~ 'ac8ac5d63b66b83b90ce41a2d4061635'::text)
               Rows Removed by Index Recheck: 10166
               Heap Blocks: exact=5247
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..132.60 rows=960 width=0) (actual time=41.966..41.970 rows=10166 loops=1)
                     Index Cond: (contents ~ 'ac8ac5d63b66b83b90ce41a2d4061635'::text)
 Planning Time: 33.703 ms
 Execution Time: 27379.786 ms
(10 rows)
```

With table splitting, it takes just 7s now:

```
$ DATABASE=postgres://postgres@127.0.0.1:5432/postgres PARALLEL=32 go run ./cmd/tablesplitgen/main.go query 'ac8ac5d63b66b83b90ce41a2d4061635' 10 200
...
0 results in 6915ms
```

### Take 3

We also tried on a query that was relatively fast before, only 500ms:

```
query: EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'fmt\.Print.*' limit 10) as e;
Timing is on.
                                                                        QUERY PLAN                                                                         
-----------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=215.15..215.16 rows=1 width=8) (actual time=557.535..557.625 rows=1 loops=1)
   ->  Limit  (cost=204.15..215.02 rows=10 width=8) (actual time=278.488..557.549 rows=10 loops=1)
         ->  Bitmap Heap Scan on files  (cost=204.15..21133.72 rows=19245 width=8) (actual time=278.479..557.364 rows=10 loops=1)
               Recheck Cond: (contents ~ 'fmt\.Print.*'::text)
               Rows Removed by Index Recheck: 345
               Heap Blocks: exact=230
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..199.33 rows=19245 width=0) (actual time=229.300..229.338 rows=228531 loops=1)
                     Index Cond: (contents ~ 'fmt\.Print.*'::text)
 Planning Time: 31.950 ms
 Execution Time: 560.100 ms
(10 rows)
```

It now executes in 177-255ms:

```
$ DATABASE=postgres://postgres@127.0.0.1:5432/postgres PARALLEL=32 go run ./cmd/tablesplitgen/main.go query 'fmt\.Print.*' 10 200
...
0 results in 191ms
```

### More exhaustive split-table benchmarking

We clearly needed more representative data, so we did a full corpus query benchmark using:

```sh
OUT=docker_stats_logs/query-run-split-index-1.log ./capture-docker-stats.sh
```

```sql
ALTER DATABASE postgres SET statement_timeout = '3600s';
```

```
./query-split-corpus.sh &> query_logs/query-run-split-index-1.log
```


#### What queries were ran?

Total number of queries ran was 350 (to save on time, we only ran a smaller number of searches - 10 per unique search - compared to our prior 20k. We found this still gave a generally reliable sample of performance):

```
$ cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq -c '.[] | {Timeout: .Timeout, Limit: .Limit, Results: .Rows, Query: .Query}' | wc -l
     350
```

Exact numbers of each query type ran:

```
$ cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq -c '.[] | {Timeout: .Timeout, Limit: .Limit, Results: .Rows, Query: .Query}' | sort | uniq -c | sort
   1 {"Timeout":false,"Limit":100,"Results":110,"Query":"fmt\\.Println"}
   1 {"Timeout":false,"Limit":1000,"Results":1011,"Query":"fmt\\.Println"}
   1 {"Timeout":false,"Limit":1000,"Results":1031,"Query":"fmt\\.Print.*"}
   1 {"Timeout":false,"Limit":1000,"Results":1031,"Query":"fmt\\.Println"}
   1 {"Timeout":false,"Limit":1000,"Results":1035,"Query":"fmt\\.Print.*"}
   1 {"Timeout":false,"Limit":1000,"Results":1573,"Query":"123456789"}
   1 {"Timeout":false,"Limit":1000,"Results":1640,"Query":"fmt\\.Println"}
   1 {"Timeout":false,"Limit":1000,"Results":1784,"Query":"fmt\\.Println"}
   1 {"Timeout":false,"Limit":1000,"Results":1995,"Query":"bytes.Buffer"}
   2 {"Timeout":false,"Limit":1000,"Results":1004,"Query":"fmt\\.Error"}
   3 {"Timeout":false,"Limit":1000,"Results":1036,"Query":"fmt\\.Print.*"}
   4 {"Timeout":false,"Limit":1000,"Results":1052,"Query":"123456789"}
   5 {"Timeout":false,"Limit":1000,"Results":1000,"Query":"123456789"}
   5 {"Timeout":false,"Limit":1000,"Results":1029,"Query":"fmt\\.Print.*"}
   6 {"Timeout":false,"Limit":1000,"Results":1000,"Query":"fmt\\.Println"}
   8 {"Timeout":false,"Limit":1000,"Results":1000,"Query":"fmt\\.Error"}
   9 {"Timeout":false,"Limit":100,"Results":100,"Query":"fmt\\.Println"}
   9 {"Timeout":false,"Limit":1000,"Results":1002,"Query":"bytes.Buffer"}
  10 {"Timeout":false,"Limit":-1,"Results":127895,"Query":"fmt\\.Error"}
  10 {"Timeout":false,"Limit":-1,"Results":22876,"Query":"fmt\\.Println"}
  10 {"Timeout":false,"Limit":-1,"Results":37319,"Query":"fmt\\.Print.*"}
  10 {"Timeout":false,"Limit":10,"Results":0,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
  10 {"Timeout":false,"Limit":10,"Results":0,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"123456789"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"bytes.Buffer"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Error"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Print.*"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"fmt\\.Println"}
  10 {"Timeout":false,"Limit":10,"Results":10,"Query":"var"}
  10 {"Timeout":false,"Limit":100,"Results":0,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
  10 {"Timeout":false,"Limit":100,"Results":0,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
  10 {"Timeout":false,"Limit":100,"Results":100,"Query":"123456789"}
  10 {"Timeout":false,"Limit":100,"Results":100,"Query":"bytes.Buffer"}
  10 {"Timeout":false,"Limit":100,"Results":100,"Query":"fmt\\.Error"}
  10 {"Timeout":false,"Limit":100,"Results":100,"Query":"fmt\\.Print.*"}
  10 {"Timeout":false,"Limit":100,"Results":100,"Query":"var"}
  10 {"Timeout":false,"Limit":1000,"Results":0,"Query":"ac8ac5d63b66b83b90ce41a2d4061635"}
  10 {"Timeout":false,"Limit":1000,"Results":0,"Query":"d97f1d3ff91543[e-f]49.8b07517548877"}
  10 {"Timeout":false,"Limit":1000,"Results":1000,"Query":"var"}
  20 {"Timeout":false,"Limit":-1,"Results":1479452,"Query":"error"}
  20 {"Timeout":false,"Limit":10,"Results":10,"Query":"error"}
  20 {"Timeout":false,"Limit":100,"Results":100,"Query":"error"}
  20 {"Timeout":false,"Limit":1000,"Results":1000,"Query":"error"}
```

#### How many timed out?

```
$ cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select (.Timeout == true)' | jq -c -s '.[]' | wc -l
       0
```

(But we should include the last single query which bricked our MacOS installation mentioned previously..)

#### CPU/memory usage

CPU usage (150% == one and a half virtual cores) during query execution as visualized by:

```sh
$ cat ./docker_stats_logs/query-run-split-index-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=7700 | jq | jp -y '..CPUPerc'
```

<img width="1144" alt="image" src="https://user-images.githubusercontent.com/3173176/108114005-79078880-7055-11eb-9f55-bc4ca65c4808.png">

Of note here is that:

2. We see an average of around ~1600% CPU used - this indicates we are actually using 100% of the CPU (the i9 has 8 physical cores, 16 virtual.) Compare this to the fact that our prior test typically only used 1 virtual CPU core.
1. Due to the way Docker projects CPU usage based on delta measurements, during great spikes of CPU utilization the recorded measurement can exceed 1600% ("16 virtual cores 100% utilized").

Memory usage in MiB during query execution as visualized by:

```sh
$ cat ./docker_stats_logs/query-run-split-index-1.log | go run ./cmd/visualize-docker-json-stats/main.go --trim-end=7700 | jq | jp -y '..MemUsageMiB'
```

<img width="1143" alt="image" src="https://user-images.githubusercontent.com/3173176/108115193-04354e00-7057-11eb-9782-8d3125c122e1.png">

Of note here is that we generally use higher amounts of memory than before, around ~380 MiB on average.

#### Other measurements

We can determine how many queries executed in under a time bucket using e.g.:

```
$ cat ./query_logs/query-run-split-index-1.log | go run ./cmd/visualize-query-log/main.go | jq '.[] | select (.ExecutionTimeMs < 25000)' | jq -c -s '.[]' | wc -l
```

Note that we did not record planning time or index rechecks for these queries.
