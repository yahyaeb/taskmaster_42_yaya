
╔════════════════════════════════════════════════════════════════════════════╗
║           SUPERVISOR: MULTIPLE INSTANCES HANDLING ARCHITECTURE             ║
╚════════════════════════════════════════════════════════════════════════════╝

1. CONFIGURATION PARSING (options.py: read_config)
═══════════════════════════════════════════════════════════════════════════

   Config File:
   ┌─────────────────────────────────────────────────┐
   │ [program:worker]                                │
   │ command=/usr/bin/python worker.py               │
   │ numprocs=3                        ← KEY!        │
   │ numprocs_start=0                  ← START INDEX │
   │ process_name=%(program_name)s:%(process_num)02d │
   └─────────────────────────────────────────────────┘

   Step 1: Extract configuration parameters
   ────────────────────────────────────────
   numprocs = 3
   numprocs_start = 0
   process_name = "%(program_name)s:%(process_num)02d"
   
   Validation: If numprocs > 1, process_name MUST contain "%(process_num)"
   
   
2. PROCESS CONFIG CREATION LOOP (options.py: lines 956-1055)
═══════════════════════════════════════════════════════════════════════════

   for process_num in range(0, 3):  # [0, 1, 2]
   
       Iteration 0:
       ───────────
       expansions = {
           'program_name': 'worker',
           'process_num': 0,
           'numprocs': 3,
           ... other environment variables ...
       }
       
       process_name = expand(template, expansions)
                    = "worker:00"
       
       pconfig = ProcessConfig(
           name="worker:00",
           command="/usr/bin/python worker.py",
           ... other params ...
       )
       programs.append(pconfig)
       
       
       Iteration 1:
       ───────────
       expansions['process_num'] = 1
       process_name = "worker:01"
       
       pconfig = ProcessConfig(name="worker:01", ...)
       programs.append(pconfig)
       
       
       Iteration 2:
       ───────────
       expansions['process_num'] = 2
       process_name = "worker:02"
       
       pconfig = ProcessConfig(name="worker:02", ...)
       programs.append(pconfig)
   
   
   Result: programs = [pconfig:worker:00, pconfig:worker:01, pconfig:worker:02]


3. PROCESS GROUP CONFIG CREATION (options.py: ProcessGroupConfig)
═══════════════════════════════════════════════════════════════════════════

   ProcessGroupConfig(
       name="worker",                    ← Group name (program section name)
       priority=999,
       process_configs=[pconfig:00, pconfig:01, pconfig:02],  ← List of 3 configs!
   )
   
   Schema: ProcessGroupConfig
   ───────────────────────────────
   {
       'name': 'worker',
       'priority': 999,
       'process_configs': [  ← KEY: List can have 1+ ProcessConfig objects
           {
               'name': 'worker:00',      ← Individual process name
               'command': '...',
               'uid': uid,
               'stdout_logfile': '...',
               'stdout_logfile_maxbytes': 50MB,
               'autostart': True,
               'autorestart': True,
               'priority': 999,
               ... other ProcessConfig attributes ...
           },
           {
               'name': 'worker:01',      ← Second instance
               ...
           },
           {
               'name': 'worker:02',      ← Third instance
               ...
           }
       ]
   }


4. PROCESS GROUP INITIALIZATION (process.py: ProcessGroupBase.__init__)
═══════════════════════════════════════════════════════════════════════════

   class ProcessGroupBase(object):
       def __init__(self, config: ProcessGroupConfig):
           self.config = config
           self.processes = {}  ← Dictionary (hash map)
           
           # KEY LOOP: Iterate over all ProcessConfigs
           for pconfig in self.config.process_configs:
               # Create Subprocess for each ProcessConfig
               process = pconfig.make_process(self)  # ProcessConfig.make_process()
               
               # Store in dict using unique process name as key
               self.processes[pconfig.name] = process
           
   Result: self.processes Dictionary
   ─────────────────────────────────
   {
       'worker:00': Subprocess(config=pconfig:00),
       'worker:01': Subprocess(config=pconfig:01),
       'worker:02': Subprocess(config=pconfig:02),
   }


5. SUBPROCESS CREATION (process.py: Subprocess / options.py: ProcessConfig.make_process)
═══════════════════════════════════════════════════════════════════════════════════════

   class ProcessConfig(Config):
       def make_process(self, group):
           from supervisor.process import Subprocess
           process = Subprocess(self)  # ← Initialize with this config
           process.group = group       ← Reference to parent ProcessGroup
           return process
   
   
   class Subprocess(object):
       def __init__(self, config: ProcessConfig):
           self.config = config        ← Reference to ProcessConfig
           self.dispatchers = {}       ← I/O handlers (stdout, stderr, stdin)
           self.pipes = {}             ← File descriptors
           self.state = ProcessStates.STOPPED
           self.pid = 0                ← PID when running (0 when stopped)
           self.group = None           ← Parent ProcessGroup
   
   Schema: Subprocess Instance
   ──────────────────────────
   {
       'config': ProcessConfig,
           'name': 'worker:00',
           'command': '/usr/bin/python worker.py',
           ...
       'state': STOPPED,
       'pid': 0,
       'dispatchers': {},  ← Populated at spawn time
       'pipes': {
           'stdout': None,
           'stderr': None,
           'stdin': None
       },
       'group': ProcessGroup instance
   }


6. STARTUP SEQUENCE (supervisord.py)
═══════════════════════════════════════════════════════════════════════════

   supervisord.main():
       ├─ ServerOptions.read_config()
       │  └─ _parse_program_configs()  ← Creates process_configs list
       │     └─ Returns ProcessGroupConfig with all 3 ProcessConfigs
       │
       ├─ add_process_group(config)
       │  └─ config.make_group()
       │     └─ ProcessGroup(config)
       │        └─ ProcessGroupBase.__init__(config)
       │           └─ Creates 3 Subprocess instances
       │
       ├─ main loop:
       │  └─ for process in group.processes.values():
       │     └─ process.transition()  ← Start each process


7. DATA STRUCTURE HIERARCHY
═══════════════════════════════════════════════════════════════════════════

   supervisord
   │
   └─ process_groups: { 'worker': ProcessGroup, ... }
      │
      └─ ProcessGroup (one per program section)
         │
         ├─ config: ProcessGroupConfig
         │  │
         │  ├─ name: 'worker'
         │  └─ process_configs: [ProcessConfig, ProcessConfig, ProcessConfig]
         │
         └─ processes: { 'worker:00': Subprocess, 'worker:01': Subprocess, ... }
            │
            ├─ Subprocess:worker:00
            │  └─ config: ProcessConfig
            │     ├─ name: 'worker:00'
            │     └─ command: '/usr/bin/python worker.py'
            │
            ├─ Subprocess:worker:01
            │  └─ config: ProcessConfig
            │     ├─ name: 'worker:01'
            │     └─ command: '/usr/bin/python worker.py'
            │
            └─ Subprocess:worker:02
               └─ config: ProcessConfig
                  ├─ name: 'worker:02'
                  └─ command: '/usr/bin/python worker.py'


8. KEY CONCEPTS
═══════════════════════════════════════════════════════════════════════════

   ✓ numprocs: How many instances to create (default=1)
   ✓ numprocs_start: Starting index for %(process_num) (default=0)
   ✓ process_name template: MUST contain %(process_num) when numprocs > 1
   ✓ ProcessConfig: Configuration for a SINGLE process instance
   ✓ ProcessGroupConfig: Configuration for a GROUP of processes (1+ instances)
   ✓ ProcessGroup: Runtime container managing N Subprocess instances
   ✓ Subprocess: Individual process instance (one OS process)
   
   Formula for naming:
   ─────────────────
   process_name = "%(program_name)s:%(process_num)02d"
   
   For 3 instances of [program:worker]:
   → worker:00, worker:01, worker:02


9. COMMAND EXAMPLES
═══════════════════════════════════════════════════════════════════════════

   supervisorctl status
   ───────────────────
   worker:00  RUNNING  pid 1234
   worker:01  RUNNING  pid 1235
   worker:02  RUNNING  pid 1236
   
   supervisorctl start worker:01
   ──────────────────────────
   → Starts only instance #1
   
   supervisorctl restart worker
   ───────────────────────────
   → Restarts all instances (worker:00, 01, 02)

