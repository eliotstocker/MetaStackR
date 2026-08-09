<h2>Simple Submodule Dependency Declarations</h2>
<pre><code class="language-yaml">name: MetaStackrConfig
submodules:
  - path: sub/auth-service
    repo: org/auth-service
  - path: sub/ui-app
    repo: org/ui-app
    dependsOn: ["sub/auth-service"]
</code></pre>
