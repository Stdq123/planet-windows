// MyGo 0.10 generated bindings invoke globalThis.mygo.callWith(method, args, options).
// Bridge lightweight test mocks that only define call(method, ...args).
type MockMyGo = {
  call?: (method: string, ...args: unknown[]) => unknown;
  callWith?: (method: string, args?: unknown[], options?: unknown) => unknown;
  [key: string]: unknown;
};

let currentMyGo: unknown = (globalThis as { mygo?: unknown }).mygo;

function wrapMockMyGo(value: unknown): unknown {
  if (!value || typeof value !== "object") return value;
  const mock = value as MockMyGo;
  if (typeof mock.call === "function" && typeof mock.callWith !== "function") {
    const call = mock.call.bind(mock);
    return Object.assign(mock, {
      callWith(method: string, args: unknown[] = []) {
        return call(method, ...args);
      },
    });
  }
  return value;
}

Object.defineProperty(globalThis, "mygo", {
  configurable: true,
  enumerable: true,
  get() {
    return currentMyGo;
  },
  set(value: unknown) {
    currentMyGo = wrapMockMyGo(value);
  },
});
