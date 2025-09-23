# redis_lock
Implementing distributed locks using Redis based on the Go language, similar to Redisson's implementation

## Examples

```go
import (
	"context"
	"github.com/redis/go-redis/v9"
	"log"
)

func main() {
	opts := &redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}

	rc := redis.NewClient(opts)
	client := NewClient(rc)
	lock := client.NewLock("my-key")
	ctx := context.Background()

	err := lock.Lock(ctx)
	if err != nil {
		log.Fatalf("Failed to lock Redis: %v\n", err)
	}

	err = lock.Unlock(ctx)
	if err != nil {
		log.Fatalf("Failed to unlock Redis: %v\n", err)
	}

}