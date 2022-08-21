# Log generator

## Description

Today most of the application architectures are built using polyglot microservices. Each microservice generate log messages of different formats.  To test performance of any APM platform it is key to have tools to generate different kind of logs, with different targets, volume of messages per time or size.

## Objectives

Here are the objectives of this log generator.

- Ability to define a different log type with possible variability.
- Ability to define size based or time-based log generation
- Ability to support different log output targets such as filesystems, http etc…
- CLI tool to trigger the log generation tracking progress and latency, throughput statistics details
- Canned log message templates.

## Log configuration

Sample configuration to define the log generation requirement.  One or more such configurations can be defined.

log-generator/conf/nginx_loggen.conf

```
# Log generator info
   Log_gen_info:
# Name of the log generator
        name: Test log generator
        
# Actual log message format that mimics application log messages.
       log_message:
       
# All the key, vals defined in the custom_info object will be wrapped part of the log request besides message.
# custom_info is optional.  If not defined only message will be sent.
       custom_info:

#To define the target of the generated log messages
       output_target:

#Size of the logs messages to be buffered in memory before writing to the target.
       buffer_size:

#Number of messages per second to be generated.
       msg_per_sec:

#Total size of the log messages to be generated per day
       size_per_day:
```

## Detailed information on each object defined above

| Object Name | Details |
| ------ | ------ |
|buffer_size| No of messages to be buffered before writing into target|
|msg_per_sec|Number of log messages per second. Default:5000|
|size_per_day|Total size of the log messages to be generated in a day Default: 100GB/day|
|size_per_day|Total size of the log messages to be generated in a day Default: 100GB/day|
| custom_info |All key, values defined in the custom_info object will be wrapped part of every log request besides message.  custom_info is optional.  If not defined only message will be sent.  All objects capture array of possible values.  Log generator will pick one of these values randomly while forming a message. |
|log_message|Log message is a collection of tokens.  Each token can be a constant literal or expression.  Expression will be evaluated to a value(s). Expressions can be functions as well.
```
Here is a sample configuration for log message format template.
log_message:
    log_message_template:
       - '$IP - $REM_USER [%d/%{MON}/%Y %z] $http_method $RESOURCE HTTP/1.1 $http_status_code $bytes_sent $http_referer $http_user_agent'
All objects capture array of possible values. Log generator will pick one of these values randomly while forming a message.
# All expressions referred in the template are defined here.
     refs:
# Possible values for IP
	    IP:
	      - 66.249.65.3
	      - 66.249.65.62
	      - 
	# Possible values for REM_USER
	    REM_USER:
	      - Bahubali
	      - JamesBond
	    http_method:
	      - GET
	      - POST
	      - DELETE
	    RESOURCE:
	      - /smiley.gif
	    http_status_code:
		  - 200
		  - 400
		  - 500
	    bytes_sent:
	      - 1023
	      - 1998
	    http_referer:
	      - https://apmmanager.snappyflow.io
	    http_user_agent:
	      - Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)
	      - Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36
	    # Bytes_sent values are generated by a function named randomInt(min_value, max_value)
	    # This function is available in loggenerator 
	    bytes_sent:  randomInt(0,1000000)
```
