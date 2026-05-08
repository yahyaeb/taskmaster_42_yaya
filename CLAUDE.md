Root:
  ├── main.go                  
  ├── go.mod                    
  ├── go.sum                  
  ├── config.yml                      
  ├── CLAUDE.md               
  ├── .context/                
  │   ├── watchdog_legacy.go
  │   ├── watchdog_legacy_test.go
  │   └── integration_test_legacy.go
  ├── internal/                
  │   ├── engine/               
  │   │   ├── executor.go
  │   │   ├── watcher.go
  │   │   ├── stopper.go
  │   │   ├── retry.go
  │   │   ├── retry_factory.go
  │   │   ├── signaler.go
  │   │   ├── builder.go
  │   │   ├── os_executor.go
  │   │   └── *_test.go        
  │   ├── config/               
  │   │   ├── spec.go
  │   │   ├── yaml_loader.go
  │   │   └── *_test.go         
  │   ├── bus/                  
  │   │   ├── event.go
  │   │   └── *_test.go
  │   └── app/                  
  │       ├── manager.go
  │       └── *_test.go         
  ├── cmd/                      
  │   ├── daemon/
  │   │   └── main.go           
  │   └── ctl/
  │       └── main.go
  ├── tmp/          
  │   └── taskmaster.sock
  │
  └── .git/

