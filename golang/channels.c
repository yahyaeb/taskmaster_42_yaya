// Simplified C equivalent of a Go channel
typedef struct {
    ProcessUpdate queue[BUFFER_SIZE];  // Array to store data
    int write_pos;                      // Where to write next
    int read_pos;                       // Where to read next
    int count;                          // How many items in queue
    pthread_mutex_t lock;               // Lock for thread safety
    pthread_cond_t not_empty;           // Signal when data arrives
    pthread_cond_t not_full;            // Signal when space available
} Channel;

// Sending (like: updates <- ProcessUpdate{...})
void channel_send(Channel *ch, ProcessUpdate data) {
    pthread_mutex_lock(&ch->lock);
    
    while (ch->count == BUFFER_SIZE) {  // Wait if full
        pthread_cond_wait(&ch->not_full, &ch->lock);
    }
    
    ch->queue[ch->write_pos] = data;    // Add to array
    ch->write_pos = (ch->write_pos + 1) % BUFFER_SIZE;  // Next position
    ch->count++;
    
    pthread_cond_signal(&ch->not_empty); // Wake up receivers
    pthread_mutex_unlock(&ch->lock);
}

// Receiving (like: update := <-updates)
ProcessUpdate channel_receive(Channel *ch) {
    pthread_mutex_lock(&ch->lock);
    
    while (ch->count == 0) {  // Wait if empty
        pthread_cond_wait(&ch->not_empty, &ch->lock);
    }
    
    ProcessUpdate data = ch->queue[ch->read_pos];
    ch->read_pos = (ch->read_pos + 1) % BUFFER_SIZE;  // Next position
    ch->count--;
    
    pthread_cond_signal(&ch->not_full);  // Wake up senders
    pthread_mutex_unlock(&ch->lock);
    
    return data;
}