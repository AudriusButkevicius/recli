recli - Reflection based CLI (command line interface) generator for Golang
--------------------------------------------------------------------------

[![GoDoc](https://godoc.org/github.com/AudriusButkevicius/recli?status.svg)](https://godoc.org/github.com/AudriusButkevicius/recli)

For a given struct, builds a set of [urfave/cli](https://github.com/urfave/cli) commands which allows you
to modify it from the command line.

Useful for generating command line clients for your application configuration that is stored in a Go struct.

## Features

* Nested struct support
* Enum/Custom complex type support via MarshalText/UnmarshalText
* Slice support, including complex types
* Slice indexing by struct field
* Map support
* Default primitive value support when adding items to slices

## Known limitations

* Adding new struct to a slice only allows setting primitive fields (use add-json as a work-around)
* Only primitive types supported for map keys and values
* No defaults for maps


## Examples

Example config

```go
type Config struct {
	Address          string `usage:"Address on which to listen"` // Description printed in -help
	AuthMode         AuthMode                                    // Enum support
	ThreadingOptions ThreadingOptions                            // Nested struct support
	Backends         []Backend                                   // Slice support
	EnvVars          map[string]string                           // Map support
}

type Backend struct {
	Hostname         string `recli:"id"`         // Constructs commands for indexing into the array based on the value of this field
	Port             int    `default:"2019"`     // Default support
	BackoffIntervals []int  `default:"10,20"`    // Slice default support
	IPAddressCached  net.IP `recli:"-" json:"-"` // Skips the field
}

type ThreadingOptions struct {
	MaxThreads int
}
```

Sample input data
```json
{
   "Address":"http://website.com",
   "AuthMode":"static",
   "ThreadingOptions":{
      "MaxThreads":10
   },
   "Backends":[
      {
         "Hostname":"backend1.com",
         "Port":1010
      },
      {
         "Hostname":"backend2.com",
         "Port":2020
      }
   ],
   "EnvVars":{
      "CC":"/usr/bin/gcc"
   }
}
```

<details>
 <summary>Full example code</summary>

```go
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/AudriusButkevicius/recli"
	"github.com/urfave/cli"
)

type Config struct {
	Address          string `usage:"Address on which to listen"` // Description printed in -help
	AuthMode         AuthMode                                    // Enum support
	ThreadingOptions ThreadingOptions                            // Nested struct support
	Backends         []Backend                                   // Slice support
	EnvVars          map[string]string                           // Map support
}

type Backend struct {
	Hostname         string `recli:"id"`         // Constructs commands for indexing into the array based on the value of this field
	Port             int    `default:"2019"`     // Default support
	BackoffIntervals []int  `default:"10,20"`    // Slice default support
	IPAddressCached  net.IP `recli:"-" json:"-"` // Skips the field
}

type ThreadingOptions struct {
	MaxThreads int
}

type AuthMode int

const (
	AuthModeStatic AuthMode = iota // default is static
	AuthModeLDAP
)

func (t AuthMode) MarshalText() ([]byte, error) {
	switch t {
	case AuthModeStatic:
		return []byte("static"), nil
	case AuthModeLDAP:
		return []byte("ldap"), nil
	}
	return nil, fmt.Errorf("unknown value: %s", t)
}

func (t *AuthMode) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "ldap":
		*t = AuthModeLDAP
	case "static":
		*t = AuthModeStatic
	default:
		return fmt.Errorf("unknown value: %s", string(bs))
	}
	return nil
}

const (
	sampleData = `
{
   "Address":"http://website.com",
   "AuthMode":"static",
   "ThreadingOptions":{
      "MaxThreads":10
   },
   "Backends":[
      {
         "Hostname":"backend1.com",
         "Port":1010
      },
      {
         "Hostname":"backend2.com",
         "Port":2020
      }
   ],
   "EnvVars":{
      "CC":"/usr/bin/gcc"
   }
}`
)

func main() {
	cfg := &Config{}

	if err := json.Unmarshal([]byte(sampleData), cfg); err != nil {
		panic(err)
	}

	cmds, err := recli.Default.Construct(cfg)
	if err != nil {
		panic(err)
	}

	dump := false

	app := cli.NewApp()
	app.Commands = cmds
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "dump",
			Destination: &dump,
		},
	}

	if err := app.Run(os.Args); err != nil {
		panic(err)
	}

	if dump {
		bs, err := json.MarshalIndent(&cfg, "", "    ")
		if err != nil {
			panic(err)
		}

		fmt.Print(string(bs))
	}
}
```
</details>

<details>
 <summary>Get a field</summary>

```bash
$ go run main.go address get
http://website.com
```
</details>

<details>
 <summary>Set a field</summary>

```bash
$ go run main.go -dump address set foo
{
    "Address": "foo",
    "AuthMode": "static",
    "ThreadingOptions": {
        "MaxThreads": 10
    },
    "Backends": [
        {
            "Hostname": "backend1.com",
            "Port": 1010,
            "BackoffIntervals": null
        },
        {
            "Hostname": "backend2.com",
            "Port": 2020,
            "BackoffIntervals": null
        }
    ],
    "EnvVars": {
        "CC": "/usr/bin/gcc"
    }
}
```
</details>

<details>
 <summary>Set a nested field</summary>

```bash
$ go run main.go -dump threading-options max-threads set 9000
{
    "Address": "http://website.com",
    "AuthMode": "static",
    "ThreadingOptions": {
        "MaxThreads": 9000
    },
    "Backends": [
        {
            "Hostname": "backend1.com",
            "Port": 1010,
            "BackoffIntervals": null
        },
        {
            "Hostname": "backend2.com",
            "Port": 2020,
            "BackoffIntervals": null
        }
    ],
    "EnvVars": {
        "CC": "/usr/bin/gcc"
    }
}
```
</details>

<details>
 <summary>Listing available slice items (with a custom slice index key)</summary>

```bash
$ go run main.go backends
NAME:
   main.exe backends -

USAGE:
   main.exe backends command [command options] [arguments...]

COMMANDS:
  ACTIONS:
     add           Add a new item to collection
     add-json      Add a new item to collection deserialised from JSON

  ITEMS:
     backend1.com
     backend2.com
     
OPTIONS:
   --help, -h  show help

```
</details>

<details>
 <summary>Deleting a slice item</summary>

```bash
$ go run main.go -dump backends backend1.com delete
{
    "Address": "http://website.com",
    "AuthMode": "static",
    "ThreadingOptions": {
        "MaxThreads": 10
    },
    "Backends": [
        {
            "Hostname": "backend2.com",
            "Port": 2020,
            "BackoffIntervals": null
        }
    ],
    "EnvVars": {
        "CC": "/usr/bin/gcc"
    }
}
```
</details>

<details>
 <summary>Adding a slice item (with defaults)</summary>

```bash
$ go run main.go -dump backends add -hostname="testback.end"
{
    "Address": "http://website.com",
    "AuthMode": "static",
    "ThreadingOptions": {
        "MaxThreads": 10
    },
    "Backends": [
        {
            "Hostname": "backend1.com",
            "Port": 1010,
            "BackoffIntervals": null
        },
        {
            "Hostname": "backend2.com",
            "Port": 2020,
            "BackoffIntervals": null
        },
        {
            "Hostname": "testback.end",
            "Port": 2019,
            "BackoffIntervals": [
                10,
                20
            ]
        }
    ],
    "EnvVars": {
        "CC": "/usr/bin/gcc"
    }
}
```
</details>

<details>
 <summary>Setting map keys</summary>

```bash
$ go run main.go -dump env-vars set GCC /usr/bin/true
{
    "Address": "http://website.com",
    "AuthMode": "static",
    "ThreadingOptions": {
        "MaxThreads": 10
    },
    "Backends": [
        {
            "Hostname": "backend1.com",
            "Port": 1010,
            "BackoffIntervals": null
        },
        {
            "Hostname": "backend2.com",
            "Port": 2020,
            "BackoffIntervals": null
        }
    ],
    "EnvVars": {
        "CC": "/usr/bin/gcc",
        "GCC": "/usr/bin/true"
    }
}
```
</details>
