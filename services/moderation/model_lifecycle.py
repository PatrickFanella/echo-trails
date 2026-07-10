import gc
from threading import RLock
from typing import Callable, Generic, TypeVar


T = TypeVar("T")


class ModelLifecycle(Generic[T]):
    """Keeps at most one inference model resident at a time."""

    def __init__(self, release: Callable[[], None] | None = None):
        self._lock = RLock()
        self._name: str | None = None
        self._model: T | None = None
        self._release = release or self._default_release

    @property
    def active_name(self) -> str | None:
        with self._lock:
            return self._name

    def activate(self, name: str, loader: Callable[[], T]) -> T:
        with self._lock:
            if self._name == name and self._model is not None:
                return self._model
            self.unload()
            self._model = loader()
            self._name = name
            return self._model

    def unload(self) -> None:
        with self._lock:
            had_model = self._model is not None
            self._model = None
            self._name = None
            if had_model:
                self._release()

    @staticmethod
    def _default_release() -> None:
        gc.collect()
        try:
            import torch

            if torch.cuda.is_available():
                torch.cuda.synchronize()
                torch.cuda.empty_cache()
        except (ImportError, RuntimeError):
            # A failed CUDA cleanup must not turn an uncertain moderation
            # outcome into an approval; the caller still fails closed.
            pass
