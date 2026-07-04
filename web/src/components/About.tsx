import { SecH } from './SecH';

// About is the "about" tab with the library summary table.
export function About() {
  return (
    <section className="view active" id="view-about">
      <SecH h="h2">About</SecH>
      <p><span className="grad-text" style={{ fontWeight: 700 }}>malcolmston/go</span> is a family of Go libraries that recreate the most-loved building blocks of the Node.js ecosystem — with the same ergonomics, on top of Go's standard library.</p>
      <SecH>The libraries</SecH>
      <table className="cmp">
        <thead><tr><th>Library</th><th>Ports</th><th>Module</th><th>Deps</th></tr></thead>
        <tbody>
          <tr><td>Express</td><td>expressjs/express</td><td className="mono">github.com/malcolmston/express</td><td>none</td></tr>
          <tr><td>Passport</td><td>jaredhanson/passport</td><td className="mono">github.com/malcolmston/passport</td><td>none</td></tr>
          <tr><td>Socket.IO</td><td>socketio/socket.io</td><td className="mono">github.com/malcolmston/socketio</td><td>none</td></tr>
          <tr><td>chalk</td><td>chalk/chalk (+inquirer, figlet)</td><td className="mono">github.com/malcolmston/chalk</td><td className="mono">x/term</td></tr>
          <tr><td>morgan</td><td>expressjs/morgan</td><td className="mono">github.com/malcolmston/morgan</td><td>none</td></tr>
        </tbody>
      </table>
      <div className="note">Not affiliated with or endorsed by the original Node.js projects or their authors. Each Go library is an independent re-implementation, released under the MIT License.</div>
    </section>
  );
}
