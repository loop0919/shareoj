;;;; Loaded by build-runtime-additions.py after the contribs and libraries.
;; The library sources are not shipped, so ASDF must never look for or rebuild a loaded system.
(dolist (system (asdf:already-loaded-systems))
  (asdf:register-immutable-system system))
(asdf:clear-source-registry)
(asdf:clear-output-translations)
;; SBCL lets the heap grow by 5% of the dynamic space (51 MiB) between collections,
;; too much under a 64 MiB limit; the worker passes a value scaled to the limit.
(push (lambda ()
        (let ((bytes (sb-ext:posix-getenv "SBCL_BYTES_CONSED_BETWEEN_GCS")))
          (when bytes
            (setf (sb-ext:bytes-consed-between-gcs) (parse-integer bytes)))))
      sb-ext:*init-hooks*)
(sb-ext:save-lisp-and-die "/opt/judge-runtimes/sbcl/bin/sbcl-judge" :executable t :save-runtime-options t)
