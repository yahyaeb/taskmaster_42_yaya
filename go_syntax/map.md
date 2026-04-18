// Declare and initialize
m := map[string]int{
    "apple": 5,
    "banana": 3,
}

// Add or update
m["orange"] = 2

// Access value
fmt.Println(m["apple"]) // 5

// Check existence
if val, ok := m["grape"]; ok {
    fmt.Println(val)
} else {
    fmt.Println("Key not found")
}   

Iterate with for key, value := range m.