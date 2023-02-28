<!--
title: "Parser"
custom_edit_url: https://github.com/netdata/netdata/blob/master/parser/README.md
sidebar_label: "Parser"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Developers/Database"
-->

# Parser

## Introduction

Generic parser that is used to register keywords and a corresponding function that will be executed when that
keyword is encountered in the command stream (either from plugins or via streaming)

To use a parser do the following:

1. Define a structure that will be used to share user state across calls (user defined `void *user`) 
2. Initialize the parser using `parser_init`
3. Register keywords with their associated callback function using `parser_add_keyword`
4. Start a loop for as long there is input (or parser_action returns error)
   1.   Fetch the next line using `parser_next` (if needed)
   2.   Process the line using `parser_action`
5. Release the parser using `parser_destroy`
6. Release the user structure

See examples in receiver.c / pluginsd_parser.c
