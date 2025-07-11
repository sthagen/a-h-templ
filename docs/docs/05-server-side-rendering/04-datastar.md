# Datastar

[Datastar](https://data-star.dev) is a hypermedia framework that is similar to [HTMX](htmx).

Datastar can selectively replace content within a web page by combining fine-grained reactive signals with SSE. It's geared primarily to real-time applications where you'd normally reach for a SPA framework such as React/Vue/Svelte.

## Usage

Using Datastar requires:

- Installation of the Datastar client-side library.
- Modifying the HTML markup to instruct the library to perform partial screen updates.

## Installation

Datastar is included with Templ components out of the box to speed up development. You can use `@datastar.ScriptCDNLatest()` or `ScriptCDNVersion(version string)` to include the latest version of the Datastar library in your HTML.

:::info
Advanced Datastar installation and usage help is covered in the user guide at https://data-star.dev.
:::

## Datastar examples using Templ

The Datastar website is built using Datastar and templ, so you can see how it works in practice.

The Datastar website contains a number of examples that demonstrate how to use Datastar. The examples are written in Go and use the templ package to generate the HTML.

See examples at https://github.com/delaneyj/datastar/tree/main/backends/go/site

This document will walk you through how to create a simple counter example using Datastar, following the [example](https://data-star.dev/examples/templ_counter) in the Datastar website.

## Counter Example

We are going to modify the [templ counter example](example-counter-application) to use Datastar.

### Frontend

First, define some HTML with two buttons. One to update a global state, and one to update a per-user state.

```templ title="components.templ"
package site

import datastar "github.com/starfederation/datastar/sdk/go"

type TemplCounterSignals struct {
	Global uint32 `json:"global"`
	User   uint32 `json:"user"`
}

templ templCounterExampleButtons() {
	<div>
		<button
			data-on-click="@post('/examples/templ_counter/increment/global')" 
		>
			Increment Global
		</button>
		<button
			data-on-click={ datastar.PostSSE('/examples/templ_counter/increment/user') }
			<!-- Alternative: Using Datastar SDK sugar--> 
		>
			Increment User
		</button>
	</div>
}

templ templCounterExampleCounts() {
	<div>
		<div>
			<div>Global</div>
			<div data-text="$global"></div>
		</div>
		<div>
			<div>User</div>
			<div data-text="$user"></div>
		</div>
	</div>
}

templ templCounterExampleInitialContents(signals TemplCounterSignals) {
	<div
		id="container"
		data-signals={ templ.JSONString(signals) }
	>
		@templCounterExampleButtons()
		@templCounterExampleCounts()
	</div>
}
```

:::tip
Note that Datastar doesn't promote the use of forms because they are ill-suited to nested reactive content. Instead, it sends all[^1] reactive state (as JSON) to the server on each request. This means far less bookkeeping and more predictable state management.
:::

:::note
`data-signals` is a special attribute that Datastar uses to merge one or more signals into the existing signals. In the example, we store $global and $user when we initially render the container. 

`data-on-click="@post('/examples/templ_counter/increment/global')"` is an attribute expression that says "When this element is clicked, send a POST request to the server to the specified URL". The `@post` is an action that is a sandboxed function that knows about things like signals.

`data-text="$global"` is an attribute expression that says "replace the contents of this element with the value of the `global` signal in the store". This is a reactive signal that will update the page when the value changes, which we'll see in a moment.
:::

### Backend

Note the use of Datastar's helpers to set up SSE.

```go title="examples_templ_counter.go"
package site

import (
	"net/http"
	"sync/atomic"

	"github.com/Jeffail/gabs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	datastar "github.com/starfederation/datastar/sdk/go"
)

func setupExamplesTemplCounter(examplesRouter chi.Router, sessionSignals sessions.Store) error {

	var globalCounter atomic.Uint32
	const (
		sessionKey = "templ_counter"
		countKey   = "count"
	)

	userVal := func(r *http.Request) (uint32, *sessions.Session, error) {
		sess, err := sessionSignals.Get(r, sessionKey)
		if err != nil {
			return 0, nil, err
		}

		val, ok := sess.Values[countKey].(uint32)
		if !ok {
			val = 0
		}
		return val, sess, nil
	}

	examplesRouter.Get("/templ_counter/data", func(w http.ResponseWriter, r *http.Request) {
		userVal, _, err := userVal(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		signals := TemplCounterSignals{
			Global: globalCounter.Load(),
			User:   userVal,
		}

		c := templCounterExampleInitialContents(signals)
		datastar.NewSSE(w, r).MergeFragmentTempl(c)
	})

	updateGlobal := func(signals *gabs.Container) {
		signals.Set(globalCounter.Add(1), "global")
	}

	examplesRouter.Route("/templ_counter/increment", func(incrementRouter chi.Router) {
		incrementRouter.Post("/global", func(w http.ResponseWriter, r *http.Request) {
			update := gabs.New()
			updateGlobal(update)

			datastar.NewSSE(w, r).MarshalAndMergeSignals(update)
		})

		incrementRouter.Post("/user", func(w http.ResponseWriter, r *http.Request) {
			val, sess, err := userVal(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			val++
			sess.Values[countKey] = val
			if err := sess.Save(r, w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			update := gabs.New()
			updateGlobal(update)
			update.Set(val, "user")

			datastar.NewSSE(w, r).MarshalAndMergeSignals(update)
		})
	})

	return nil
}
```

The `atomic.Uint32` type stores the global state. The `userVal` function is a helper that retrieves the user's session state. The `updateGlobal` function increments the global state.

:::note
In this example, the global state is stored in RAM and will be lost when the web server reboots. To support load-balanced web servers and stateless function deployments, consider storing the state in a data store such as [NATS KV](https://docs.nats.io/using-nats/developer/develop_jetstream/kv).
:::

### Per-user session state

In an HTTP application, per-user state information is partitioned by an HTTP cookie. Cookies that identify a user while they're using a site are known as "session cookies". When the HTTP handler receives a request, it can read the session ID of the user from the cookie and retrieve any required state.

### Signal-only patching

Since the page's elements aren't changing dynamically, we can use the `MarshalAndMergeSignals` function to send only the signals that have changed. This is a more efficient way to update the page without even needing to send HTML fragments.

:::tip
Datastar will merge updates to signals similar to a JSON merge patch. This means you can do dynamic partial updates to the store and the page will update accordingly. [Gabs](https://pkg.go.dev/github.com/Jeffail/gabs/v2#section-readme) is used here to handle dynamic JSON in Go.

[^1]: You can control the data sent to the server by prefixing local signals with `_`. This will prevent them from being sent to the server on every request.
