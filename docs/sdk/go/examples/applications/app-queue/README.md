# Queue Demo – Using HydrAIDE Go SDK

This example demonstrates how to build a simple queue using the HydrAIDE Go SDK.

It shows you how to:

Insert data (Treasures) into a HydrAIDE catalog with a future expireAt timestamp

Read back only the expired Treasures from that catalog

This pattern is ideal for implementing auto-expiring queue entries, message buffers, or scheduled task dispatching.

## 🔧 Prerequisites

To run this project, you need a HydrAIDE test instance running either locally or remotely. Before starting, make sure to:

* Review the [HydrAIDE Installation Guide](/docs/how-to-install-hydraide.md) to set up your instance
* Have access to the necessary TLS certificate files

### Required Environment Variables

Before launching the app, define the following variables in your environment:

```bash
HYDRA_HOST=localhost:5444
HYDRA_CERT=/path/to/ca.crt
```

* `HYDRA_HOST`: Address of your HydrAIDE server
* `HYDRA_CERT`: Path to the client TLS certificate file

---

## 📁 Project Structure

```text
app-queue/
├── main.go
├── appserver/
│   └── ...
├── services/
│   └── queue/
│       ├── model_catalog_queue.go
│       └── ...
└── utils/
    ├── hydraidehelper/
    ├── panichandler/
    └── repo/
```

### `main.go`

This is the main entry point of the application.

---

### `appserver/`

This folder contains the runnable application logic. It executes real operations against HydrAIDE using a preconfigured setup so you can test queue functionality in your local environment. It logs each message to the console for verification.

---

### `services/`

Each service lives in its own subfolder under `services/`. This is where the HydrAIDE models and related logic live.

For example:

* `model_catalog_queue.go`: a **catalog model** named `queue`
* `model_profile_user.go`: a **profile model** named `user`

We always wrap HydrAIDE models in a service layer. While it’s technically possible to call models directly from an API or CLI, business logic should live in this layer.

#### Naming Convention:

* `model_catalog_queue`: means it's a HydrAIDE **catalog-type** model named `queue`
* `model_profile_user`: means it's a HydrAIDE **profile-type** model named `user`

This naming helps make the model's role and behavior immediately clear.

---

### `utils/`

This folder provides reusable components and helpers:

* `hydraidehelper/`: utility functions to support HydrAIDE usage
* `panichandler/`: centralized panic handling for this demo app
* `repo/`: simplified setup for HydrAIDE connections

> 💡 Tip: Feel free to copy these utility packages into your own project to accelerate development.

---

## ✅ Summary

This demo helps you:

* Connect to a TLS-secured HydrAIDE instance
* Build and register catalog/profile models
* Structure your services around business logic
* Log and inspect queue behavior locally

By following this layout and naming pattern, your HydrAIDE-based services remain clear, testable, and production-ready.
