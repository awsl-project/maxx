# Writes coreml.mlmodel and coreml.json, the CoreML program coreml.go
# runs AstrLink Guard's encoder with, from the int8 model's own weights:
#
#   python3 -I coreml_gen.py <the model's folder>   (coremltools 9, numpy, onnx)
#
# The program is bert.go's network after the embeddings, for one window of
# 512 tokens: x [1, 512, 384] (the embeddings, layer-normed) and mask
# [1, 1, 1, 512] (0 for a token, -1e4 for padding) in, logits [1, 512, 21]
# out, in float16. Its weights are the int8 ones dequantized; they are not
# kept here but written on each Mac from the model downloaded
# (coremlWeights), into weight.bin at the offsets coreml.json gives, and
# coreml.json's sha256 is that file's as coremltools writes it.
import hashlib
import json
import os
import shutil
import sys
import tempfile

import coremltools as ct
import numpy as np
import onnx
from coremltools.converters.mil import Builder as mb
from coremltools.proto import Model_pb2
from onnx import numpy_helper

S, H, NH, D = 512, 384, 12, 32

m = onnx.load(os.path.join(sys.argv[1], "model_int8.onnx"))
T = {i.name: numpy_helper.to_array(i) for i in m.graph.initializer}


def mat(p):
    q = T[p + "_quantized"].astype(np.float32)
    z = T[p + "_zero_point"].astype(np.float32)
    return ((q - z) * T[p + "_scale"]).T.copy()  # [n, k], as linear takes it


# Each weight by the key coremlWeights knows it by.
W = {}
for i in range(12):
    p, mm = f"inner.bert.encoder.layer.{i}.", 1299 + 16 * i
    W.update({
        f"l{i}_qkv_weight": mat(f"Attention_{i}_qkv_weight"),
        f"l{i}_qkv_bias": T[f"Attention_{i}_qkv_bias"],
        f"l{i}_ao_weight": mat(f"onnx::MatMul_{mm}"),
        f"l{i}_ao_bias": T[p + "attention.output.dense.bias"],
        f"l{i}_ln1_gamma": T[p + "attention.output.LayerNorm.weight"],
        f"l{i}_ln1_beta": T[p + "attention.output.LayerNorm.bias"],
        f"l{i}_up_weight": mat(f"onnx::MatMul_{mm + 1}"),
        f"l{i}_up_bias": T[p + "intermediate.dense.bias"],
        f"l{i}_smooth_y": T[f"r15_smooth_inv_s_{i}"],
        f"l{i}_dn_weight": mat(f"onnx::MatMul_{mm + 2}"),
        f"l{i}_dn_bias": T[p + "output.dense.bias"],
        f"l{i}_ln2_gamma": T[p + "output.LayerNorm.weight"],
        f"l{i}_ln2_beta": T[p + "output.LayerNorm.bias"],
    })
W["logits_weight"] = mat("onnx::MatMul_1478")
W["logits_bias"] = T["inner.classifier.bias"]


@mb.program(input_specs=[mb.TensorSpec(shape=(1, S, H)), mb.TensorSpec(shape=(1, 1, 1, S))])
def prog(x, mask):
    def heads(t):
        return mb.transpose(x=mb.reshape(x=t, shape=[1, S, NH, D]), perm=[0, 2, 1, 3])

    for i in range(12):
        def lin(t, k):
            return mb.linear(x=t, weight=W[f"l{i}_{k}_weight"], bias=W[f"l{i}_{k}_bias"], name=f"l{i}_{k}")

        def norm(t, k):
            return mb.layer_norm(x=t, axes=[-1], gamma=W[f"l{i}_{k}_gamma"], beta=W[f"l{i}_{k}_beta"],
                                 epsilon=1e-5, name=f"l{i}_{k}")

        q, k, v = mb.split(x=lin(x, "qkv"), num_splits=3, axis=-1)
        q = mb.mul(x=heads(q), y=np.float32(1 / np.sqrt(D)))
        s = mb.add(x=mb.matmul(x=q, y=heads(k), transpose_y=True), y=mask)
        c = mb.matmul(x=mb.softmax(x=s, axis=-1), y=heads(v))
        c = mb.reshape(x=mb.transpose(x=c, perm=[0, 2, 1, 3]), shape=[1, S, H])
        x = norm(mb.add(x=lin(c, "ao"), y=x), "ln1")
        h = mb.mul(x=mb.gelu(x=lin(x, "up"), mode="EXACT"), y=W[f"l{i}_smooth_y"], name=f"l{i}_smooth")
        x = norm(mb.add(x=lin(h, "dn"), y=x), "ln2")
    return mb.linear(x=x, weight=W["logits_weight"], bias=W["logits_bias"], name="logits")


ml = ct.convert(prog, convert_to="mlprogram", compute_precision=ct.precision.FLOAT16,
                minimum_deployment_target=ct.target.macOS13)
tmp = tempfile.mkdtemp()
pkg = os.path.join(tmp, "m.mlpackage")
ml.save(pkg)
data = os.path.join(pkg, "Data", "com.apple.CoreML")
spec = Model_pb2.Model()
spec.ParseFromString(open(os.path.join(data, "model.mlmodel"), "rb").read())
spec.description.metadata.Clear()  # the date it was made, coremltools' version

blobs = []
fn = spec.mlProgram.functions["main"]
for op in fn.block_specializations[fn.opset].operations:
    v = op.attributes["val"] if op.type == "const" else None
    if v is None or not v.HasField("blobFileValue"):
        continue
    name = op.outputs[0].name
    assert name.endswith("_0_to_fp16"), name
    key = name[:-len("_0_to_fp16")]
    assert key in W and W[key].size == np.prod([d.constant.size for d in v.type.tensorType.dimensions]), key
    blobs.append([key, v.blobFileValue.offset])
assert sorted(k for k, _ in blobs) == sorted(W), "a weight is not in the program"

here = os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(here, "coreml.mlmodel"), "wb") as f:
    f.write(spec.SerializeToString(deterministic=True))  # map fields in order
wb = open(os.path.join(data, "weights", "weight.bin"), "rb").read()
shutil.rmtree(tmp)
with open(os.path.join(here, "coreml.json"), "w") as f:
    f.write('{"size": %d, "sha256": "%s", "blobs": [\n' % (len(wb), hashlib.sha256(wb).hexdigest()))
    f.write(",\n".join(json.dumps(b) for b in blobs))
    f.write("\n]}\n")
