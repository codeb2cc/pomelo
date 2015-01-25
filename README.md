Pomelo - Index Service based on Suffix Array
============================================


Usage
-----
    Usage: pomelo COMMAND [OPTIONS]

    Command:
      -console -index=PATH
      -web [-index=PATH] [-http=:8080] [-procs=2]
      -build -src=PATH -dst=PATH [-max-length=120] [-min-value=1000]

####Build Index from Stdin

      -build -src=@stdin -dst=PATH [-max-length=120] [-min-value=1000]


Data Source Format
------------------

    STRING

or

    STRING\tWEIGHT


Web API
-------

#### List avaiable indexes

    GET /indexes/

#### Query

    GET /index/{INDEX KEY}/?q=hello
    GET /index/{INDEX KEY}/?q=hello&q=world

#### Load(localhost *ONLY*)

    POST /index/    path=PATH&key=KEY

#### Unload(localhost *ONLY*)

    DELETE /index/{INDEX KEY}/

