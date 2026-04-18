# Taskmaster

**Summary:** The goal of this project is to make a job control daemon, with features similar to supervisor.

**Version:** 3.1

---

## Chapter III: Goals

Your job here is to make a fully-fledged job control daemon. A pretty good example of this would be [supervisor](http://supervisord.org/).

For the sake of keeping it simple, your program will not run as root, and does not HAVE to be a daemon. It will be started via shell, and do its job while providing a control shell to the user.

---

## Chapter IV: General Instructions

### IV.1 Language constraints

You are free to use any programming language you prefer. External libraries are allowed only for parsing configuration files, for using the readline library or an equivalent, and for implementing the client/server bonus (if you choose to do so). Other than that, you are strictly limited to your language's standard library.

### IV.2 Defense session

For the defense session, be prepared to:

- Demonstrate that your program correctly implements each and every required feature, by running it with a configuration file you will provide.
- Have your program tested by your grader in various ways, including, but not limited to, manually killing supervised processes, trying to launch processes that never start correctly, launching processes that generate lots of output, etc...

---

## Chapter V: Mandatory Part

This project needs to be done on a Virtual Machine.

Your program must be able to start jobs as child processes, and keep them alive, restarting them if necessary. It must also know at all times if these processes are alive or dead (This must be accurate).

Information on which programs must be started, how, how many, if they must be restarted, etc... will be contained in a configuration file, the format of which is up to you (YAML is a good idea, for example, but use whatever you want). This configuration must be loaded at launch, and must be reloadable, while taskmaster is running, by sending a SIGHUP to it. When it is reloaded, your program is expected to effect all the necessary changes to its run state (Removing programs, adding some, changing their monitoring conditions, etc ...), but it must NOT de-spawn processes that haven't been changed in the reload.

Your program must have a logging system that logs events to a local file (When a program is started, stopped, restarted, when it dies unexpectedly, when the configuration is reloaded, etc ...)

When started, your program must remain in the foreground, and provide a control shell to the user. It does not HAVE to be a fully-fledged shell like 42sh, but it must be at the very least usable (Line editing, history... completion would also be nice). Take inspiration from supervisor's control shell, supervisorctl.

> **Note:** You can use any tools you want to set up your host virtual machine.

### Control Shell Requirements

This shell must at least allow the user to:

- See the status of all the programs described in the config file ("status" command)
- Start / stop / restart programs
- Reload the configuration file without stopping the main program
- Stop the main program

### Configuration File Requirements

The configuration file must allow the user to specify the following, for each program that is to be supervised:

- The command to use to launch the program
- The number of processes to start and keep running
- Whether to start this program at launch or not
- Whether the program should be restarted always, never, or on unexpected exits only
- Which return codes represent an "expected" exit status
- How long the program should be running after it's started for it to be considered "successfully started"
- How many times a restart should be attempted before aborting
- Which signal should be used to stop (i.e. exit gracefully) the program
- How long to wait after a graceful stop before killing the program
- Options to discard the program's stdout/stderr or to redirect them to files
- Environment variables to set before launching the program
- A working directory to set before launching the program
- An umask to set before launching the program

---

## Chapter VI: Bonus part

You are encouraged to implement any supplemental feature you think your project will benefit from. You will get points for it if it is correctly implemented and at least vaguely useful.

Here are some ideas to get you started:

- Privilege de-escalation on launch (Needs to be started as root).
- Client/server architecture to allow for two separate programs: A daemon, that does the actual job control, and a control program, that provides a shell for the user, and communicates with the daemon over UNIX or TCP sockets. (Very much like supervisord and supervisorctl)
- More advanced logging/reporting facilities (Alerts via email/http/syslog/etc...)
- Allow the user to "attach" a supervised process to its console, much in the way that tmux or screen do, then "detach" from it and put it back in the background.

---

## Chapter VII: Appendix

### VII.1 Example configuration file

This is what a configuration file for your taskmaster COULD look like:

```yaml
programs:
  nginx:
    cmd: "/usr/local/bin/nginx -c /etc/nginx/test.conf"
    numprocs: 1
    umask: 022
    workingdir: /tmp
    autostart: true
    autorestart: unexpected
    exitcodes:
      - 0
      - 2
    startretries: 3
    starttime: 5
    stopsignal: TERM
    stoptime: 10
    stdout: /tmp/nginx.stdout
    stderr: /tmp/nginx.stderr
    env:
      STARTED_BY: taskmaster
      ANSWER: 42

  vogsphere:
    cmd: "/usr/local/bin/vogsphere-worker --no-prefork"
    numprocs: 8
    umask: 077
    workingdir: /tmp
    autostart: true
    autorestart: unexpected
    exitcodes: 0
    startretries: 3
    starttime: 5
    stopsignal: USR1
    stoptime: 10
    stdout: /tmp/vgsworker.stdout
    stderr: /tmp/vgsworker.stderr
```

### VII.2 Trying out supervisor

supervisor is available on PyPI as a Python package. To try it out, the simplest way is to create a virtualenv in your home, activate it, and then install supervisor with `pip install supervisor`. You may have to install python before, it's available on Homebrew.

You can then make a configuration file to manage one or two programs, launch `supervisord -c myconfigfile.conf`, then interact with it using `supervisorctl`.

Keep in mind that supervisor is a mature, feature-rich program, and that what you must do with taskmaster is less complicated, so you should just see it as a source of inspiration. For example, supervisor offers the control shell on a separate process that communicates with the main program via a UNIX-domain socket, while you only have to provide a control shell in the main program.

If you have doubts about what behaviour your program should have in a certain case, or what meaning to give to some options... well, when in doubt, do it like supervisor does, you can't go wrong.

---

## Chapter VIII: Submission and peer correction

Submit your work on your GiT repository as usual. Only the work on your repository will be graded.

Good luck to all and don't forget your author file!
