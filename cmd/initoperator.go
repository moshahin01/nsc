/*
 * Copyright 2018 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nsc/cmd/store"
	"github.com/spf13/cobra"
)

func createInitClusterCmd() *cobra.Command {
	var params InitClusterParams

	var cmd = &cobra.Command{
		Use:    "operator",
		Hidden: !show,
		Short:  "initializes the current directory for operator configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := params.Validate(cmd); err != nil {
				return err
			}
			if err := params.Run(args); err != nil {
				return err
			}

			d, err := params.kp.Seed()
			if err != nil {
				return fmt.Errorf("error reading seed: %v", err)
			}

			if params.generate {
				d := FormatKeys("operator", params.publicKey, string(d))
				if err := Write("--", d); err != nil {
					return err
				}
			} else {
				cmd.Printf("Success! - operator created %q\n", params.publicKey)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&params.generate, "generate-nkeys", "", false, "generate operator nkey")
	cmd.Flags().StringVarP(&params.name, "name", "", "", "name for the operator")
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("private-key")

	return cmd
}

func init() {
	initCmd.AddCommand(createInitClusterCmd())
}

type InitClusterParams struct {
	generate  bool
	name      string
	kp        nkeys.KeyPair
	publicKey string
	dir       string
}

func (p *InitClusterParams) Validate(cmd *cobra.Command) error {
	var err error

	p.name = strings.TrimSpace(p.name)
	if strings.HasSuffix(p.name, " ") {
		return fmt.Errorf("names cannot have spaces: %q", p.name)
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting cwd: %v", err)
	}

	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	if len(infos) != 0 {
		return fmt.Errorf("directory %q is not empty", dir)
	}

	if p.generate && cmd.Flag("private-key").Changed {
		return errors.New("provide only one of --generate-nkeys or --private-key")
	}

	if p.generate {
		p.kp, err = nkeys.CreateOperator()
		if err != nil {
			return fmt.Errorf("error creating operator key: %v", err)
		}
	}

	if cmd.Flag("private-key").Changed {
		p.kp, err = GetSeed()
		if err != nil {
			return fmt.Errorf("error parsing seed: %v", err)
		}
		pk, err := p.kp.PublicKey()
		if err != nil {
			return fmt.Errorf("error reading public key: %v", err)
		}

		if !nkeys.IsValidPublicOperatorKey(pk) {
			return fmt.Errorf("%q is not a valid operator key", string(pk))
		}
	}

	return nil
}

func (p *InitClusterParams) Run(args []string) error {
	if err := p.CreateStore(); err != nil {
		return err
	}
	if err := p.CreateDirs(); err != nil {
		return err
	}
	if err := p.WriteJwt(); err != nil {
		return err
	}

	return nil
}

func (p *InitClusterParams) CreateStore() error {
	var err error
	d, err := p.kp.PublicKey()
	if err != nil {
		return fmt.Errorf("error reading or creating operator public key: %v", err)
	}
	p.publicKey = string(d)

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting cwd: %v", err)
	}
	_, err = store.CreateStore(dir, p.publicKey, "operator", p.name)
	return err
}

func (p *InitClusterParams) CreateDirs() error {
	// make some directories
	s, err := getStore()
	if err != nil {
		return err
	}
	cp := filepath.Join(s.Dir, "clusters")
	return os.MkdirAll(cp, 0700)
}

func (p *InitClusterParams) WriteJwt() error {
	c := jwt.NewOperatorClaims(p.publicKey)
	c.Name = p.name
	d, err := c.Encode(p.kp)
	if err != nil {
		return err
	}

	fn := filepath.Join(p.dir, fmt.Sprintf("%s.jwt", p.name))
	return ioutil.WriteFile(fn, []byte(d), 0666)
}
