/* eslint-disable */
/* tslint:disable */


const PACKAGE_VERSION = '3.0.3'
const INTEGRITY_CHECKSUM = '5cd5cf8b54c3a90f82960cedcd637772'
const IS_MOCKED_RESPONSE = Symbol('isMockedResponse')

const activeClientIds = new Set()
const pendingRequests = new Map()

addEventListener('install', function () {
  self.skipWaiting()
})

addEventListener('activate', function (event) {
  event.waitUntil(self.clients.claim())
})

addEventListener('message', function (event) {
  const clientId = Reflect.get(event.source || {}, 'id')

  if (!clientId || !self.clients) {
    return
  }

  event.waitUntil(
    (async () => {
      if (event.data === 'CLIENT_CLOSE') {
        const allClients = await self.clients.matchAll({
          type: 'window',
        })

        activeClientIds.delete(clientId)

        const pending = pendingRequests.get(clientId)
        if (pending != null && pending.size > 0) {
          await Promise.allSettled(pending)
        }
        pendingRequests.delete(clientId)

        const remainingClients = allClients.filter((client) => {
          return client.id !== clientId
        })

        if (remainingClients.length === 0) {
          await self.registration.unregister()
        }

        const client = await self.clients.get(clientId)

        if (client != null) {
          await sendToClient(client, {
            type: 'CLIENT_CLOSED',
          })
        }

        return
      }

      const client = await self.clients.get(clientId)

      if (!client) {
        return
      }

      switch (event.data) {
        case 'KEEPALIVE_REQUEST': {
          await sendToClient(client, {
            type: 'KEEPALIVE_RESPONSE',
          })
          break
        }

        case 'INTEGRITY_CHECK_REQUEST': {
          await sendToClient(client, {
            type: 'INTEGRITY_CHECK_RESPONSE',
            payload: {
              packageVersion: PACKAGE_VERSION,
              checksum: INTEGRITY_CHECKSUM,
            },
          })
          break
        }

        case 'MOCK_ACTIVATE': {
          activeClientIds.add(clientId)

          await sendToClient(client, {
            type: 'MOCKING_ENABLED',
            payload: {
              client: {
                id: client.id,
                frameType: client.frameType,
              },
            },
          })
          break
        }
      }
    })(),
  )
})

addEventListener('fetch', function (event) {
  if (
    event.request.cache === 'only-if-cached' &&
    event.request.mode !== 'same-origin'
  ) {
    return
  }

  if (activeClientIds.size === 0) {
    return
  }

  const requestId = crypto.randomUUID()
  event.respondWith(handleRequest(event, requestId))
})

async function handleRequest(event, requestId) {
  const client = await resolveMainClient(event)
  const requestCloneForEvents = event.request.clone()

  const responsePromise = getResponse(event, client, requestId)

  if (client != null) {
    let pending = pendingRequests.get(client.id)

    if (pending == null) {
      pendingRequests.set(client.id, (pending = new Set()))
    }

    pending.add(responsePromise)
    responsePromise
      .finally(() => pending.delete(responsePromise))
      .catch(() => {})
  }

  let response

  try {
    response = await responsePromise
  } catch (error) {
    if (client && activeClientIds.has(client.id)) {
      sendToClient(client, {
        type: 'REQUEST_ERROR',
        payload: {
          request: {
            id: requestId,
          },
          error: serializeError(error),
        },
      })
    }

    throw error
  }

  if (client && activeClientIds.has(client.id)) {
    const serializedRequest = await serializeRequest(requestCloneForEvents)

    const isEventStreamResponse = response.headers
      .get('content-type')
      ?.toLowerCase()
      .startsWith('text/event-stream')

    const responseClone = isEventStreamResponse ? null : response.clone()

    sendToClient(
      client,
      {
        type: 'RESPONSE',
        payload: {
          isMockedResponse: IS_MOCKED_RESPONSE in response,
          request: {
            id: requestId,
            ...serializedRequest,
          },
          response: {
            type: response.type,
            status: response.status,
            statusText: response.statusText,
            headers: Object.fromEntries(response.headers.entries()),
            body: responseClone ? responseClone.body : null,
          },
        },
      },
      responseClone && responseClone.body
        ? [serializedRequest.body, responseClone.body]
        : [],
    )
  }

  return response
}

async function resolveMainClient(event) {
  const client = await self.clients.get(event.clientId)

  if (activeClientIds.has(event.clientId)) {
    return client
  }

  if (client?.frameType === 'top-level') {
    return client
  }

  const allClients = await self.clients.matchAll({
    type: 'window',
  })

  return allClients
    .filter((client) => {
      return client.visibilityState === 'visible'
    })
    .find((client) => {
      return activeClientIds.has(client.id)
    })
}

async function getResponse(event, client, requestId) {
  const requestClone = event.request.clone()

  function passthrough(data) {
    const headers = new Headers()
    const requestHeaders = data?.request?.headers

    if (Array.isArray(requestHeaders)) {
      for (const [name, value] of requestHeaders) {
        headers.append(name, value)
      }
    } else {
      for (const [name, value] of requestClone.headers) {
        headers.append(name, value)
      }
    }

    const acceptHeader = headers.get('accept')
    if (acceptHeader) {
      const values = acceptHeader.split(',').map((value) => value.trim())
      const filteredValues = values.filter(
        (value) => value !== 'msw/passthrough',
      )

      if (filteredValues.length > 0) {
        headers.set('accept', filteredValues.join(', '))
      } else {
        headers.delete('accept')
      }
    }

    return fetch(requestClone, { headers })
  }

  if (!client) {
    return passthrough()
  }

  if (!activeClientIds.has(client.id)) {
    return passthrough()
  }

  const serializedRequest = await serializeRequest(event.request)
  const clientMessage = await sendToClient(
    client,
    {
      type: 'REQUEST',
      payload: {
        id: requestId,
        ...serializedRequest,
      },
    },
    [serializedRequest.body],
  )

  switch (clientMessage.type) {
    case 'MOCK_RESPONSE': {
      return respondWithMock(clientMessage.data, event)
    }

    case 'PASSTHROUGH': {
      return passthrough(clientMessage.data)
    }
  }

  return passthrough()
}

function serializeError(error) {
  if (error instanceof Error) {
    return {
      name: error.name,
      message: error.message,
    }
  }

  return {
    name: 'Error',
    message: String(error),
  }
}

function sendToClient(client, message, transferrables = []) {
  return new Promise((resolve, reject) => {
    const channel = new MessageChannel()

    channel.port1.onmessage = (event) => {
      if (event.data && event.data.error) {
        return reject(event.data.error)
      }

      resolve(event.data)
    }

    client.postMessage(message, [
      channel.port2,
      ...transferrables.filter(Boolean),
    ])
  })
}

async function respondWithMock(response, event) {
  if (response.status === 0) {
    return Response.error()
  }

  let body = response.body

  if (event.request.mode === 'navigate' && body instanceof ReadableStream) {
    body = await new Response(body).arrayBuffer()
  }

  const mockedResponse = new Response(body, response)

  Reflect.defineProperty(mockedResponse, IS_MOCKED_RESPONSE, {
    value: true,
    enumerable: true,
  })

  return mockedResponse
}

async function serializeRequest(request) {
  return {
    url: request.url,
    mode: request.mode,
    method: request.method,
    headers: Object.fromEntries(request.headers.entries()),
    cache: request.cache,
    credentials: request.credentials,
    destination: request.destination,
    integrity: request.integrity,
    redirect: request.redirect,
    referrer: request.referrer,
    referrerPolicy: request.referrerPolicy,
    body: await request.arrayBuffer(),
    keepalive: request.keepalive,
  }
}
