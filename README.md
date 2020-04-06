# Guestbook Operator with Operator SDK.

This is hobby project which creates k8s operator for [guestbook](https://kubernetes.io/docs/tutorials/stateless-application/guestbook/) application.

I will update this repo step by step and tag it from 00. I will try to explain how to reach this status. Master branch has everything in it.

# tag stage_00: initialize empty operator project
- Here is my env details versions (links to install guide)
  - [GO: 1.14](https://golang.org/doc/install)
  - [operator sdk: v0.16.0](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md)
  - [Git : 2.15.0](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
  - [docker : 19.03.5](https://docs.docker.com/docker-for-mac/install/), 
  - [kubectl: 1.17](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- To create this operator I am following [this guide.](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md) 
- As a first step ran the command 
  
  `operator-sdk new guestbook --repo=github.com/gurunath/guestbook` 

  I given `--repo` option as I am not using default go path to keep my go code.
- This will generate basic scaffolding for our operator. I will not go in detail, many other blogs explained it very nicely.

# tag stage_01: create new CRD
- To create ne CRD, run the command in created directory
  
  `operator-sdk add api --api-version=gurunath/v1alpha1 --kind=Guestbook`
  
  This will scaffold the Guestbook resource API under `/pkg/apis/gurunath/v1alpha1/`
- **Define the specs and status**: 
  - in first round, we will just create deployments if not present. for this we need to get `redis slave image`, `redis master image` and `guestbook image`
  - add these properties in [guestbook_types.go](./pkg/apis/gurunath/v1alpha1/guestbook_types.go) file. 
    - GuestbookSpec
      ```
      RedisSlave  string `json:"redisSlave"`
      RedisMaster string `json:"redisMaster"`
      GuestBook   string `json:"guestBook"`
      ```
      we will add validations in next steps
    - GuestbookStatus
      ```
      RedisSlavePods  []string `json:"redisSlavesPods"`
      RedisMasterPods []string `json:"redisMasterPods"`
      GuestBookPods   []string `json:"guestBookPods"`
      ```
  - for simplicity not added any validations till now
  - After updating file always run the following command to update the generated code for that resource type
  
    `operator-sdk generate k8s`
  - now Update / generate CRD file `operator-sdk generate crds`
  - in next steps we will write our controller.
- **Add new Controller**
  - Add a new Controller to the project that will watch and reconcile the Guestbook resource with bellow command
  
    `operator-sdk add controller --api-version=gurunath/v1alpha1 --kind=Guestbook`

    This will scaffold a new Controller implementation under `pkg/controller/guestbook`
  - comments are added in the controller please go through the file.

- **Deployment**
  - generate docker image for the operator with command `operator-sdk build gurunath/guestbook-operator:0.0.1` where `gurunath/guestbook-operator` is public docker repo and `0.0.1` is the tag
  - after docker image build push the image to repo.
  - create deployment, role, role binding and service account for the operator. Operator-sdk provide manifests files in `deploy` directory 

- **Demo
  - Deployed this operator as [Katakoda scenario](https://www.katacoda.com/gurunath.sane@gmail.com/scenarios/guestbook-operator) please take a look. 