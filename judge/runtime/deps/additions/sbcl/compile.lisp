;;;; sbcl-judge --script compile.lisp SOURCE FASL
;;;; Warnings still give a usable fasl. SBCL also writes one after a compile-time error
;;;; (the form fails when loaded), so an error must fail the compile here.
(destructuring-bind (source fasl) (rest sb-ext:*posix-argv*)
  (let ((failed nil)
        (output (sb-ext:parse-native-namestring fasl)))
    (handler-bind ((sb-c:compiler-error (lambda (condition)
                                          (declare (ignore condition))
                                          (setf failed t))))
      ;; :unspecific keeps compile-file from adding .fasl to a name without a type.
      (unless (compile-file source :verbose nil
                                   :output-file (make-pathname :type (or (pathname-type output) :unspecific)
                                                               :defaults output))
        (setf failed t)))
    (when failed
      (sb-ext:exit :code 1))))
