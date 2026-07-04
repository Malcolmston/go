// Command integration is a runnable example that wires several of the
// malcolmston/go libraries together on a single net/http server:
//
//   - express   — HTTP routing for the JSON API
//   - socketio  — realtime chat over WebSocket / polling
//   - morgan    — request logging around everything
//
// It builds against the sibling submodules via the repository's go.work
// workspace, demonstrating that the libraries compose through the standard
// http.Handler interface. Run it with:
//
//	go run ./examples/integration
package main

import (
	"log"
	"net/http"

	"github.com/malcolmston/express"
	"github.com/malcolmston/morgan"
	socketio "github.com/malcolmston/socketio"
)

func main() {
	// 1. HTTP routes with Express.
	app := express.New()
	app.Get("/api/hello", func(req *express.Request, res *express.Response, next express.Next) {
		res.JSON(map[string]any{"msg": "hi", "who": req.Query("name")})
	})

	// 2. Realtime chat with Socket.IO.
	io := socketio.New()
	io.OnConnection(func(s *socketio.Socket) {
		s.Join("general")
		s.On("chat", func(args []any) []any {
			io.To("general").Emit("chat", args...) // broadcast to the room
			return nil
		})
		s.On("ping", func(args []any) []any {
			return []any{"pong"} // acknowledgement reply
		})
	})

	// 3. io.Handler intercepts /socket.io/ and delegates the rest to Express.
	handler := io.Handler(app)

	// 4. Wrap the whole thing in morgan request logging.
	logged := morgan.New(handler, morgan.Dev, morgan.Config{})

	log.Println("listening on :3000")
	log.Fatal(http.ListenAndServe(":3000", logged))
}
