# Measuring the performance of pg_trgm

- `cmd/githubscrape` contains a script that fetches the top 1,000 repositories for any language.
- `top_repos/` contains URLs to the top 1,000 repositories for a given language. In total, 20,578 repositories.
- `./clone_corpus.sh` clones all 20,578 repositories (concurrently.)

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

This will take around ~8 hours on a 2020 Macbook Pro i9 w/ 16G memory.

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

Of particular note is that, again, almost 100% of the time was spent with a single CPU core maxed out and the vast majority of the CPU in `Idle` state (red):

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

## Query performance

SQL:

```
select id from files where contents ~ $1 LIMIT $2;
```

10:31pm: start executing queries

We start with a small LIMIT of `10`:

| Query | Limit | Results | Time |
|-------|-------|---------|------|
| `error` | 10 | 10 | 15ms |
| `error` | 10 | 10 | 7ms |
| `error` | 10 | 10 | 6ms |
| `error` | 10 | 10 | 9ms |
| `error` | 10 | 10 | 8ms |
| `fmt.Error` | 10 | 10 | 3000ms |
| `fmt.Error` | 10 | 10 | 756ms |
| `fmt.Error` | 10 | 10 | 763ms |
| `fmt.Error` | 10 | 10 | 747ms |
| `error` | 10 | 10 | 8ms |
| `fmt.Errorf` | 10 | 10 | 488ms |
| `fmt.Errorf` | 10 | 10 | 414ms |
| `fmt.Errorf` | 10 | 10 | 413ms |
| `fmt.Println` | 10 | 10 | 216ms |
| `fmt.Println` | 10 | 10 | 268ms |
| `fmt.Println` | 10 | 10 | 287ms |

We raise the limit to 1,000:

| Query | Limit | Results | Time |
|-------|-------|---------|------|
| `fmt.Println` | 1000 | 1000 | 1816ms |
| `fmt.Println` | 1000 | 1000 | 477ms |
| `fmt.Println` | 1000 | 1000 | 471ms |
| `error` | 1000 | 1000 | 1931ms |
| `error` | 1000 | 1000 | 2493ms |
| `error` | 1000 | 1000 | 800ms |
| `error` | 1000 | 1000 | 2328ms |
| `var` | 1000 | 1000 | 1330ms |
| `var` | 1000 | 1000 | 1072ms |
| `fmt.Error` | 1000 | 1000 | 1233ms |
| `fmt.Error` | 1000 | 1000 | 1100ms |

We drop the limit entirely:

```
postgres=# select count(id) from (select id from files where contents ~ 'fmt.Error') as e;
 count  
--------
 127900
(1 row)

Time: 693035.477 ms (11:33.035)
```

(10:52pm)

```
postgres=# select count(id) from (select id from files where contents ~ '^.*fmt\.Error.*&') as e;
^CCancel request sent
ERROR:  canceling statement due to user request
Time: 1534.340 ms (00:01.534)
postgres=# select count(id) from (select id from files where contents ~ '^.*fmt\.Error.*$') as e;
 count  
--------
 127897
(1 row)

Time: 531329.544 ms (08:51.330)
```

11:03pm

And we add back a LIMIT:

```
postgres=# select count(id) from (select id from files where contents ~ 'bytes.Buffer' LIMIT 10) as e;
 count 
-------
    10
(1 row)

Time: 9478.462 ms (00:09.478)
```

```
postgres=# select count(id) from (select id from files where contents ~ 'bytes.Buffer' LIMIT 10) as e;
 count 
-------
    10
(1 row)

Time: 2499.148 ms (00:02.499)
```

### Investigating causes of slowness

We can see clearly that bitmap heap scans take the most time, even on smaller queries for relatively rare trigrams:

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'bytes.Buffer' LIMIT 10) as e;
                                                                       QUERY PLAN                                                                       
--------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=73.66..73.67 rows=1 width=8) (actual time=2349.134..2349.183 rows=1 loops=1)
   ->  Limit  (cost=62.44..73.54 rows=10 width=8) (actual time=2050.648..2349.111 rows=10 loops=1)
         ->  Bitmap Heap Scan on files  (cost=62.44..1127.40 rows=960 width=8) (actual time=2050.637..2348.991 rows=10 loops=1)
               Recheck Cond: (contents ~ 'bytes.Buffer'::text)
               Rows Removed by Index Recheck: 2263
               Heap Blocks: exact=1068
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..62.20 rows=960 width=0) (actual time=269.440..269.444 rows=240080 loops=1)
                     Index Cond: (contents ~ 'bytes.Buffer'::text)
 Planning Time: 7.834 ms
 Execution Time: 2352.454 ms
(10 rows)

Time: 2360.962 ms (00:02.361)
```

Raising `work_mem` from `16MB` to `8GB` does not appear to help with this type of query, the first query after postgres starts is incredibly slow:

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'bytes.Buffer' LIMIT 10) as e;
                                                                        QUERY PLAN                                                                        
----------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=73.66..73.67 rows=1 width=8) (actual time=40055.427..40055.493 rows=1 loops=1)
   ->  Limit  (cost=62.44..73.54 rows=10 width=8) (actual time=39239.585..40054.848 rows=10 loops=1)
         ->  Bitmap Heap Scan on files  (cost=62.44..1127.40 rows=960 width=8) (actual time=39239.575..40054.691 rows=10 loops=1)
               Recheck Cond: (contents ~ 'bytes.Buffer'::text)
               Rows Removed by Index Recheck: 2263
               Heap Blocks: exact=1068
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..62.20 rows=960 width=0) (actual time=1808.915..1808.931 rows=240080 loops=1)
                     Index Cond: (contents ~ 'bytes.Buffer'::text)
 Planning Time: 43163.457 ms
 Execution Time: 40076.800 ms
(10 rows)
```

Subsequent attempts are still ~2.4s:

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'bytes.Buffer' LIMIT 10) as e;
                                                                       QUERY PLAN                                                                       
--------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=73.66..73.67 rows=1 width=8) (actual time=2440.732..2440.783 rows=1 loops=1)
   ->  Limit  (cost=62.44..73.54 rows=10 width=8) (actual time=2157.211..2440.690 rows=10 loops=1)
         ->  Bitmap Heap Scan on files  (cost=62.44..1127.40 rows=960 width=8) (actual time=2157.177..2440.528 rows=10 loops=1)
               Recheck Cond: (contents ~ 'bytes.Buffer'::text)
               Rows Removed by Index Recheck: 2263
               Heap Blocks: exact=1068
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..62.20 rows=960 width=0) (actual time=289.955..289.959 rows=240080 loops=1)
                     Index Cond: (contents ~ 'bytes.Buffer'::text)
 Planning Time: 5.030 ms
 Execution Time: 2443.597 ms
(10 rows)
```

It is also worth noting that some queries can devolve into full index rechecks, if they match many documents (the `\b` here makes us recheck 235,891 documents):

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ '\bbytes.Buffer\b' LIMIT 10) as e;
                                                                        QUERY PLAN                                                                        
----------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=90.16..90.17 rows=1 width=8) (actual time=1044322.378..1044322.434 rows=1 loops=1)
   ->  Limit  (cost=78.94..90.04 rows=10 width=8) (actual time=1044320.897..1044320.924 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=78.94..1143.90 rows=960 width=8) (actual time=1044320.359..1044320.378 rows=0 loops=1)
               Recheck Cond: (contents ~ '\bbytes.Buffer\b'::text)
               Rows Removed by Index Recheck: 235891
               Heap Blocks: exact=121443
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..78.70 rows=960 width=0) (actual time=1010.510..1010.521 rows=235891 loops=1)
                     Index Cond: (contents ~ '\bbytes.Buffer\b'::text)
 Planning Time: 5.431 ms
 Execution Time: 1044726.375 ms
(10 rows)
```

This can also occur sometimes in non-intuitive fashions, for example a query that matches no results can be quite slow if it matches a number of common trigrams across the documents:

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa' LIMIT 10) as e;
                                                                      QUERY PLAN                                                                       
-------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=144.06..144.07 rows=1 width=8) (actual time=62122.593..62122.624 rows=1 loops=1)
   ->  Limit  (cost=132.84..143.94 rows=10 width=8) (actual time=62122.570..62122.589 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=132.84..1197.80 rows=960 width=8) (actual time=62122.538..62122.550 rows=0 loops=1)
               Recheck Cond: (contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa'::text)
               Rows Removed by Index Recheck: 2468
               Heap Blocks: exact=1968
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..132.60 rows=960 width=0) (actual time=910.442..910.446 rows=2468 loops=1)
                     Index Cond: (contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa'::text)
 Planning Time: 8.868 ms
 Execution Time: 62123.915 ms
(10 rows)

postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa' LIMIT 10) as e;
                                                                      QUERY PLAN                                                                       
-------------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=144.06..144.07 rows=1 width=8) (actual time=14894.472..14894.503 rows=1 loops=1)
   ->  Limit  (cost=132.84..143.94 rows=10 width=8) (actual time=14894.460..14894.479 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=132.84..1197.80 rows=960 width=8) (actual time=14894.439..14894.451 rows=0 loops=1)
               Recheck Cond: (contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa'::text)
               Rows Removed by Index Recheck: 2468
               Heap Blocks: exact=1968
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..132.60 rows=960 width=0) (actual time=118.807..118.811 rows=2468 loops=1)
                     Index Cond: (contents ~ 'asodijowijaoiwjdaoiwjdowaijdwoaidjwaoidjwa'::text)
 Planning Time: 18.044 ms
 Execution Time: 14897.061 ms
(10 rows)
```

It can be quite difficult to even craft queries that do not require rechecks:

```
postgres=# EXPLAIN ANALYZE select count(id) from (select id from files where contents ~ 'a1%e\$1j\.k9e&k\^k1g3g4g5g6h6j23kj1' LIMIT 10) as e;
                                                                     QUERY PLAN                                                                     
----------------------------------------------------------------------------------------------------------------------------------------------------
 Aggregate  (cost=144.06..144.07 rows=1 width=8) (actual time=1874.255..1874.287 rows=1 loops=1)
   ->  Limit  (cost=132.84..143.94 rows=10 width=8) (actual time=1874.240..1874.260 rows=0 loops=1)
         ->  Bitmap Heap Scan on files  (cost=132.84..1197.80 rows=960 width=8) (actual time=1874.229..1874.242 rows=0 loops=1)
               Recheck Cond: (contents ~ 'a1%e\$1j\.k9e&k\^k1g3g4g5g6h6j23kj1'::text)
               Rows Removed by Index Recheck: 367
               Heap Blocks: exact=287
               ->  Bitmap Index Scan on files_contents_trgm_idx  (cost=0.00..132.60 rows=960 width=0) (actual time=14.385..14.389 rows=367 loops=1)
                     Index Cond: (contents ~ 'a1%e\$1j\.k9e&k\^k1g3g4g5g6h6j23kj1'::text)
 Planning Time: 6.285 ms
 Execution Time: 1874.698 ms
(10 rows)
```

### Resource utilization during queries

CPU usage:

<img width="599" alt="image" src="https://user-images.githubusercontent.com/3173176/106710640-7f0c5c80-65b3-11eb-962a-1f220727b57d.png">

Memory pressure:

<img width="598" alt="image" src="https://user-images.githubusercontent.com/3173176/106710661-8c294b80-65b3-11eb-8b2f-c453d9d78380.png">

Memory usage:

<img width="602" alt="image" src="https://user-images.githubusercontent.com/3173176/106710685-99463a80-65b3-11eb-89aa-42d1ce7698bd.png">

Memory swap:

<img width="600" alt="image" src="https://user-images.githubusercontent.com/3173176/106710715-ab27dd80-65b3-11eb-989e-a60ec9211b1c.png">

Disk read/write:

<img width="600" alt="image" src="https://user-images.githubusercontent.com/3173176/106710749-baa72680-65b3-11eb-8a01-1add9e49fc24.png">

