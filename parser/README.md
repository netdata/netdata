#### Introduction

The parser will be used to process streaming and plugins input as well as metadata

Usage

1. Define a structure that will be used to share user state across calls 
1. Initialize the parser using `parser_init`
2. Register keywords and associated callback function using `parser_add_keyword`
3. Register actions on the keywords 
4. Start a loop until EOF
   1.  Fetch the next line using `parser_next`
   2.  Process the line using `parser_action` 
       1. The registered callbacks are executed to parse the input
       2. The registered action for the callback is called for processing
4. Release the parser using `parser_destroy`
5. Release the user structure

#### Functions

----
##### parse_init(RRDHOST *host, void *user, void *input, int flags)

Initialize an internal parser with the specified user defined data structure that will be shared across calls.

Input
- Host
  - The host this parser will be dealing with. For streaming with SSL enabled for this host
- user
  - User defined structure that is passed in all the calls
- input
  - Where the parser will get the input from
- flags
  - flags to define processing on the input

Output
- A parser structure
  


----
##### parse_push(PARSER *parser, char *line)

Push a new line for processing

Input

- parser
  - The parser object as returned by the `parser_init`
- line
  - The new line to process
    

Output
- The line will be injected into the stream and will be the next one to be processed
  
Returns
- 0 line added
- 1 error detected
  
----   
##### parse_add_keyword(PARSER *parser, char *keyword, keyword_function callback_function)

The function will add callbacks for keywords. The callback function is defined as

`typedef PARSER_RC (*keyword_function)(char **, void *);`

Input

- parser
  - The parser object as returned by the `parser_init`
- keyword
  - The keyword to register
- keyword_function
  - The callback that will handle the keyword processing
    * The callback function should return one of the following
      * PARSER_RC_OK -- Callback was successful (continue with other callbacks)
      * PARSER_RC_STOP -- Stop processing callbacks (return OK)
      * PARSER_RC_ERROR -- Callback failed, exit

Output
- The corresponding keyword and callback will be registered
  
Returns
- 0 maximum callbacks already registered for this keyword
- > 0 which is the number of callbacks associated with this keyword.

   
----
##### parser_next(PARSER *parser)
Return the next item to parse

Input
- parser
  - The parser object as returned by the `parser_init`
  
Output
- The parser will store internally the next item to parse

Returns
- 0 Next item fetched successfully
- 1 No more items to parse

----
##### parser_action(PARSER *parser, char *input)
Return the next item to parse

Input
- parser
  - The parser object as returned by the `parser_init`
- input
  - Process the input specified instead of using the internal buffer
  
Output
- The current keyword will be processed by calling all the registered callbacks

Returns
- 0 Callbacks called successfully
- 1 Failed

----
##### parser_destroy(PARSER *parser)
Cleanup a previously allocated parser

Input
- parser
  - The parser object as returned by the `parser_init`
  
Output
- The parser is deallocated

Returns
- none
  
----
##### parser_recover_input(PARSER *parser)
Cleanup a previously allocated parser

Input
- parser
  - The parser object as returned by the `parser_init`
  
Output
- The parser is deallocated

Returns
- none
