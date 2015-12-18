/*
 *
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 */

#include "proton/data.hpp"
#include "proton/message.hpp"
#include "proton/error.hpp"
#include "proton/link.hpp"
#include "proton/delivery.hpp"
#include "proton/message.h"
#include "proton/sender.hpp"
#include "proton/receiver.hpp"
#include "proton/message_id.hpp"
#include "proton/delivery.h"
#include "msg.hpp"
#include "proton_bits.hpp"

#include <string>
#include <assert.h>

namespace proton {

// impl exists so body is pre-constructed in reference mode (prevent it creating its own data)
// for all message ctors.
message::impl::impl() : msg(0), body(0) {}

message::impl::~impl() {
    if (msg) {
        body.ref(0);            // Clear the reference.
        pn_message_free(msg);
    }
}

// Lazy construct the message.
pn_message_t* message::pn_msg() const {
    if (!impl_.msg) impl_.msg = pn_message();
    return impl_.msg;
}

message::message() {}

message::message(const message &m) { *this = m; }

#if PN_HAS_CPP11
message::message(message &&m) { swap(m); }
#endif

message::~message() {}

void message::swap(message& m) { std::swap(impl_.msg, m.impl_.msg); }

message& message::operator=(const message& m) {
    // TODO aconway 2015-08-10: more efficient pn_message_copy function
    std::string data;
    m.encode(data);
    decode(data);
    return *this;
}

void message::clear() { if (impl_.msg) pn_message_clear(impl_.msg); }

namespace {
void check(int err) {
    if (err) throw error(error_str(err));
}
} // namespace

void message::id(const message_id& id) { pn_message_set_id(pn_msg(), id.scalar_.atom_); }

namespace {
inline message_id from_pn_atom(const pn_atom_t& v) {
  switch (v.type) {
    case PN_ULONG:
      return message_id(amqp_ulong(v.u.as_ulong));
    case PN_UUID:
      return message_id(amqp_uuid(v.u.as_uuid));
    case PN_BINARY:
      return message_id(amqp_binary(v.u.as_bytes));
    case PN_STRING:
      return message_id(amqp_string(v.u.as_bytes));
    default:
      return message_id();
  }
}
}

message_id message::id() const {
    return from_pn_atom(pn_message_get_id(pn_msg()));
}

void message::user_id(const std::string &id) {
    check(pn_message_set_user_id(pn_msg(), pn_bytes(id)));
}

std::string message::user_id() const {
    return str(pn_message_get_user_id(pn_msg()));
}

void message::address(const std::string &addr) {
    check(pn_message_set_address(pn_msg(), addr.c_str()));
}

std::string message::address() const {
    const char* addr = pn_message_get_address(pn_msg());
    return addr ? std::string(addr) : std::string();
}

void message::subject(const std::string &s) {
    check(pn_message_set_subject(pn_msg(), s.c_str()));
}

std::string message::subject() const {
    const char* s = pn_message_get_subject(pn_msg());
    return s ? std::string(s) : std::string();
}

void message::reply_to(const std::string &s) {
    check(pn_message_set_reply_to(pn_msg(), s.c_str()));
}

std::string message::reply_to() const {
    const char* s = pn_message_get_reply_to(pn_msg());
    return s ? std::string(s) : std::string();
}

void message::correlation_id(const message_id& id) {
    data(pn_message_correlation_id(pn_msg())).copy(id.scalar_);
}

message_id message::correlation_id() const {
    return from_pn_atom(pn_message_get_correlation_id(pn_msg()));
}

void message::content_type(const std::string &s) {
    check(pn_message_set_content_type(pn_msg(), s.c_str()));
}

std::string message::content_type() const {
    const char* s = pn_message_get_content_type(pn_msg());
    return s ? std::string(s) : std::string();
}

void message::content_encoding(const std::string &s) {
    check(pn_message_set_content_encoding(pn_msg(), s.c_str()));
}

std::string message::content_encoding() const {
    const char* s = pn_message_get_content_encoding(pn_msg());
    return s ? std::string(s) : std::string();
}

void message::expiry_time(amqp_timestamp t) {
    pn_message_set_expiry_time(pn_msg(), t.milliseconds);
}
amqp_timestamp message::expiry_time() const {
    return amqp_timestamp(pn_message_get_expiry_time(pn_msg()));
}

void message::creation_time(amqp_timestamp t) {
    pn_message_set_creation_time(pn_msg(), t);
}
amqp_timestamp message::creation_time() const {
    return pn_message_get_creation_time(pn_msg());
}

void message::group_id(const std::string &s) {
    check(pn_message_set_group_id(pn_msg(), s.c_str()));
}

std::string message::group_id() const {
    const char* s = pn_message_get_group_id(pn_msg());
    return s ? std::string(s) : std::string();
}

void message::reply_to_group_id(const std::string &s) {
    check(pn_message_set_reply_to_group_id(pn_msg(), s.c_str()));
}

std::string message::reply_to_group_id() const {
    const char* s = pn_message_get_reply_to_group_id(pn_msg());
    return s ? std::string(s) : std::string();
}

bool message::inferred() const { return pn_message_is_inferred(pn_msg()); }

void message::inferred(bool b) { pn_message_set_inferred(pn_msg(), b); }

const value& message::body() const { return impl_.body.ref(pn_message_body(pn_msg())); }
value& message::body() { return impl_.body.ref(pn_message_body(pn_msg())); }

// MAP CACHING: the properties, annotations and instructions maps can either be
// encoded in the pn_message pn_data_t structures OR decoded as C++ map members
// of the message but not both. At least one of the pn_data_t or the map member
// is always empty, the non-empty one is the authority.

// Decode a map on demand
template<class M> M& get_map(pn_message_t* msg, pn_data_t* (*get)(pn_message_t*), M& map) {
    data d(get(msg));
    if (map.empty() && !d.empty()) {
        d.decoder() >> rewind() >> map;
        d.clear();              // The map member is now the authority.
    }
    return map;
}

// Encode a map if necessary.
template<class M> M& put_map(pn_message_t* msg, pn_data_t* (*get)(pn_message_t*), M& map) {
    data d(get(msg));
    if (d.empty() && !map.empty()) {
        d.encoder() << map;
        map.clear();        // The encoded pn_data_t  is now the authority.
    }
    return map;
}

message::property_map& message::properties() {
    return get_map(pn_msg(), pn_message_properties, properties_);
}

const message::property_map& message::properties() const {
    return get_map(pn_msg(), pn_message_properties, properties_);
}


message::annotation_map& message::annotations() {
    return get_map(pn_msg(), pn_message_annotations, annotations_);
}

const message::annotation_map& message::annotations() const {
    return get_map(pn_msg(), pn_message_annotations, annotations_);
}


message::annotation_map& message::instructions() {
    return get_map(pn_msg(), pn_message_instructions, instructions_);
}

const message::annotation_map& message::instructions() const {
    return get_map(pn_msg(), pn_message_instructions, instructions_);
}

void message::encode(std::string &s) const {
    put_map(pn_msg(), pn_message_properties, properties_);
    put_map(pn_msg(), pn_message_annotations, annotations_);
    put_map(pn_msg(), pn_message_instructions, instructions_);
    size_t sz = s.capacity();
    if (sz < 512) sz = 512;
    while (true) {
        s.resize(sz);
        int err = pn_message_encode(pn_msg(), const_cast<char*>(s.data()), &sz);
        if (err) {
            if (err != PN_OVERFLOW)
                check(err);
        } else {
            s.resize(sz);
            return;
        }
        sz *= 2;
    }
}

std::string message::encode() const {
    std::string data;
    encode(data);
    return data;
}

void message::decode(const std::string &s) {
    properties_.clear();
    annotations_.clear();
    instructions_.clear();
    check(pn_message_decode(pn_msg(), s.data(), s.size()));
}

void message::decode(proton::link link, proton::delivery delivery) {
    std::string buf;
    buf.resize(delivery.pending());
    ssize_t n = link.recv(const_cast<char *>(buf.data()), buf.size());
    if (n != ssize_t(buf.size())) throw error(MSG("link read failure"));
    clear();
    decode(buf);
    link.advance();
}

bool message::durable() const { return pn_message_is_durable(pn_msg()); }
void message::durable(bool b) { pn_message_set_durable(pn_msg(), b); }

duration message::ttl() const { return duration(pn_message_get_ttl(pn_msg())); }
void message::ttl(duration d) { pn_message_set_ttl(pn_msg(), d.milliseconds); }

uint8_t message::priority() const { return pn_message_get_priority(pn_msg()); }
void message::priority(uint8_t d) { pn_message_set_priority(pn_msg(), d); }

bool message::first_acquirer() const { return pn_message_is_first_acquirer(pn_msg()); }
void message::first_acquirer(bool b) { pn_message_set_first_acquirer(pn_msg(), b); }

uint32_t message::delivery_count() const { return pn_message_get_delivery_count(pn_msg()); }
void message::delivery_count(uint32_t d) { pn_message_set_delivery_count(pn_msg(), d); }

int32_t message::sequence() const { return pn_message_get_group_sequence(pn_msg()); }
void message::sequence(int32_t d) { pn_message_set_group_sequence(pn_msg(), d); }

}
